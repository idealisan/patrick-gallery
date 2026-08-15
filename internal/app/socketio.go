package app

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Socket.IO (Engine.IO v4) realtime endpoint for official Immich clients.
//
// The bundled SPA uses the plain `/api/events` websocket; the official
// Immich mobile/web apps instead speak Socket.IO (an Engine.IO transport with
// a Socket.IO message layer on top). This file implements enough of that
// protocol to let the official apps connect and receive realtime events:
//
//   - Engine.IO handshake (open packet with sid + ping interval/timeout)
//   - websocket transport (the path the apps use; works with or without a
//     prior polling handshake / sid)
//   - polling transport handshake + long-poll GET (best-effort; events are
//     delivered, so the app can proceed to the websocket upgrade)
//   - Socket.IO connect/ack and event packets, including ping/pong
//   - event names matching Immich's gateway (onAssetUpload, onAssetUpdate,
//     onAssetTrash, onAssetDelete, onAlbumUpdate, onAlbumDelete,
//     onAlbumAddAssets, onAlbumRemoveAssets, ...)
//
// It is wired to the same in-memory EventBus as /api/events, so every server
// mutation fans out to both the SPA and official clients.

const (
	sioPingInterval = 25 * time.Second
	sioPingTimeout  = 20 * time.Second
)

// socketIOOpen builds the Engine.IO "open" packet payload.
func socketIOOpen(sid string) string {
	return `{"sid":"` + sid + `","upgrades":["websocket"],"pingInterval":25000,"pingTimeout":20000,"maxPayload":1000000}`
}

// socketIOEventName maps an immich-go internal event type to the Socket.IO
// event name the official Immich v3.1.0 clients subscribe to. The upstream
// gateway uses snake_case names (on_asset_delete, on_asset_update, ...); an
// earlier build emitted camelCase (onAssetDelete) which the clients never
// matched, so realtime push was silently dead. See docs/COMPAT_FINDINGS.md.
func socketIOEventName(typ string) string {
	switch typ {
	case "asset.create":
		return "on_upload_success"
	case "asset.update":
		return "on_asset_update"
	case "asset.restore":
		return "on_asset_restore"
	case "asset.trash":
		return "on_asset_trash"
	case "asset.delete":
		return "on_asset_delete"
	case "asset.hide":
		return "on_asset_hidden"
	case "asset.stack":
		return "on_asset_stack_update"
	case "album.create", "album.update":
		return "on_album_update"
	case "album.delete":
		return "on_album_delete"
	case "album.addAssets":
		return "on_album_add_assets"
	case "album.removeAssets":
		return "on_album_remove_assets"
	case "user.delete":
		return "on_user_delete"
	case "config.update":
		return "on_config_update"
	case "server.version":
		return "on_server_version"
	case "new_release":
		return "on_new_release"
	case "session.delete":
		return "on_session_delete"
	case "person.thumbnail":
		return "on_person_thumbnail"
	case "notification":
		return "on_notification"
	default:
		return ""
	}
}

// socketIOPayload reshapes an internal EventBus payload into the shape the
// official client expects for the given event: delete/trash/restore events
// carry {"ids":[...]} internally but the client handlers take a bare string[]
// (the payload is forwarded straight to the local store as the id list), while
// other events keep their object payload. This transform only affects the
// Socket.IO wire format; the plain /api/events websocket keeps the internal
// shape untouched (so the bundled SPA is unaffected).
func socketIOPayload(typ string, payload map[string]any) any {
	switch typ {
	case "asset.delete", "asset.trash", "asset.restore":
		if ids, ok := payload["ids"].([]string); ok {
			return ids
		}
	}
	return payload
}

// socketIOPacket encodes an engine message carrying a Socket.IO event.
func socketIOPacket(typ string, name string, payload map[string]any) string {
	arr := []any{name, socketIOPayload(typ, payload)}
	b, err := json.Marshal(arr)
	if err != nil {
		return ""
	}
	// Engine.IO "message" (4) + Socket.IO "event" (2) + JSON array.
	return "42" + string(b)
}

// handleSocketIO routes Engine.IO requests by transport.
func (a *App) handleSocketIO(c *gin.Context) {
	if strings.EqualFold(c.Query("transport"), "websocket") {
		a.socketIOWebsocket(c)
		return
	}
	a.socketIOPolling(c)
}

// socketIOWebsocket runs the Engine.IO/Socket.IO protocol over a single
// websocket connection.
func (a *App) socketIOWebsocket(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var wmu sync.Mutex
	writePkt := func(s string) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(s))
	}

	sid := newUUID()
	writePkt("0" + socketIOOpen(sid))

	sub := a.bus.subscribe()
	defer a.bus.unsubscribe(sub)

	// Writer: forward bus events + heartbeats to the client.
	ping := time.NewTicker(sioPingInterval)
	defer ping.Stop()
	go func() {
		for {
			select {
			case e, ok := <-sub:
				if !ok {
					return
				}
				if name := socketIOEventName(e.Type); name != "" {
					if pkt := socketIOPacket(e.Type, name, e.Payload); pkt != "" {
						writePkt(pkt)
					}
				}
			case <-ping.C:
				writePkt("2") // Engine.IO ping
			}
		}
	}()

	// Reader: handle ping/pong and the Socket.IO connect handshake.
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		pkt := string(data)
		if len(pkt) == 0 {
			continue
		}
		switch pkt[0] {
		case '2': // Engine.IO ping -> pong
			writePkt("3")
		case '3': // pong (ignore)
		case '4': // Engine.IO message -> Socket.IO packet
			if len(pkt) >= 2 {
				switch pkt[1] {
				case '0': // Socket.IO connect -> ack
					writePkt("40")
				case '1': // disconnect
					return
				}
				// event/ack (client->server) are intentionally ignored
			}
		}
	}
}

// socketIOPolling implements the Engine.IO polling transport: a handshake
// that returns a sid, a long-poll GET that waits for the next event, and a
// POST that the client uses to send packets (acknowledged, not processed).
func (a *App) socketIOPolling(c *gin.Context) {
	sid := c.Query("sid")
	if sid == "" {
		// Handshake: mint a sid and return the open packet.
		sid = newUUID()
		c.Data(http.StatusOK, "text/plain; charset=UTF-8", []byte("0"+socketIOOpen(sid)+"\n"))
		return
	}

	if c.Request.Method == http.MethodPost {
		// Client sends its packets here (e.g. the "40" connect). We accept and
		// acknowledge; the app will then upgrade to the websocket transport.
		_, _ = io.Copy(io.Discard, c.Request.Body)
		c.Status(http.StatusOK)
		return
	}

	// Long-poll GET: block until an event is available (or timeout).
	sub := a.bus.subscribe()
	defer a.bus.unsubscribe(sub)
	select {
	case e, ok := <-sub:
		if !ok {
			c.Data(http.StatusOK, "text/plain; charset=UTF-8", []byte("6\n"))
			return
		}
		if name := socketIOEventName(e.Type); name != "" {
			if pkt := socketIOPacket(e.Type, name, e.Payload); pkt != "" {
				c.Data(http.StatusOK, "text/plain; charset=UTF-8", []byte(pkt+"\n"))
				return
			}
		}
		c.Data(http.StatusOK, "text/plain; charset=UTF-8", []byte("6\n"))
	case <-time.After(sioPingInterval):
		c.Data(http.StatusOK, "text/plain; charset=UTF-8", []byte("6\n"))
	}
}
