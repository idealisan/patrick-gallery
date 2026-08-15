package app

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Event is a realtime notification pushed to connected clients over the
// websocket sync endpoint. Type is a dot-namespaced subject (e.g.
// "asset.create"); Payload carries the minimal data a client needs to
// reconcile its local view.
type Event struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// EventBus is an in-memory pub/sub for server events. It is single-user /
// private-LAN oriented: every authenticated subscriber receives every event
// (there is no per-user fan-out yet, which matches the single-owner scope and
// keeps the implementation free of a heavier message broker).
type EventBus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func newEventBus() *EventBus {
	return &EventBus{subs: make(map[chan Event]struct{})}
}

func (b *EventBus) subscribe() chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *EventBus) unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
}

// publish delivers e to every subscriber without blocking; a slow client
// whose buffer is full simply drops the (best-effort, realtime) event.
func (b *EventBus) publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

var wsUpgrader = websocket.Upgrader{
	// Private-LAN / single-user: accept connections from any origin.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleEventsWS upgrades the request to a websocket and streams realtime
// events to the client. It mirrors Immich's realtime sync (Socket.IO) intent
// with a plain RFC6455 websocket: clients receive asset/album/user change
// events and reconcile their UI. The official mobile app uses Socket.IO
// specifically, so this endpoint is primarily for the bundled SPA (and any
// client that speaks plain websocket); it is the Go port's realtime layer.
func (a *App) handleEventsWS(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch := a.bus.subscribe()
	defer a.bus.unsubscribe(ch)

	_ = conn.WriteJSON(gin.H{"type": "init", "payload": gin.H{"ok": true}})

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	go func() {
		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return
				}
				if err := conn.WriteJSON(e); err != nil {
					return
				}
			case <-ping.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Reader only consumes control frames (pong / close); application
	// messages from the client are intentionally ignored.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// emit publishes an event to all connected websocket clients. It is a no-op
// when the bus is nil (defensive) and never blocks the calling handler.
func (a *App) emit(typ string, payload map[string]any) {
	if a.bus == nil {
		return
	}
	a.bus.publish(Event{Type: typ, Payload: payload})
}

// emitAsset is a small convenience for the common asset.* event shape.
func (a *App) emitAsset(typ string, id string) {
	a.emit(typ, map[string]any{"id": id})
}
