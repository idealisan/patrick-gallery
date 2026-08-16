package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestSocketIOHandshakeAndEvents validates the Engine.IO v4 / Socket.IO
// wire protocol end-to-end (hand-rolled client, no third-party Socket.IO
// client). It mirrors what the official Immich apps send:
//  1. open websocket to /api/socket.io/?EIO=4&transport=websocket
//  2. receive Engine.IO "open" packet (type 0) with a sid
//  3. send Socket.IO "connect" (40); receive connect ack (40)
//  4. server streams an event as Engine.IO message + Socket.IO event (42[...])
func TestSocketIOHandshakeAndEvents(t *testing.T) {
	app, r, token := newTestServer(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/socket.io/?EIO=4&transport=websocket"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial /api/socket.io: %v", err)
	}
	defer conn.Close()

	// 1) open packet
	_, openMsg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read open: %v", err)
	}
	if openMsg[0] != '0' {
		t.Fatalf("expected Engine.IO open (0), got %q", string(openMsg))
	}
	var open struct {
		Sid string `json:"sid"`
	}
	if err := json.Unmarshal(openMsg[1:], &open); err != nil {
		t.Fatalf("open json: %v", err)
	}
	if open.Sid == "" {
		t.Fatal("open packet missing sid")
	}

	// 2) connect -> ack. The server also pushes the on_server_version event
	// right after the open packet, so keep reading until we see the Socket.IO
	// connect ack (a packet starting with "40"); ignore pings and the
	// version announcement in between.
	if err := conn.WriteMessage(websocket.TextMessage, []byte("40")); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	ack := ""
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read connect ack: %v", err)
		}
		s := string(msg)
		if s == "2" { // Engine.IO ping -> pong
			_ = conn.WriteMessage(websocket.TextMessage, []byte("3"))
			continue
		}
		if strings.HasPrefix(s, "40") {
			ack = s
			break
		}
		// otherwise (e.g. 42 on_server_version) ignore and keep reading
	}
	if !strings.HasPrefix(ack, "40") {
		t.Fatalf("expected connect ack starting with '40', got %q", ack)
	}

	// 3) emit an event and expect a Socket.IO event packet (42[...])
	got := make(chan string, 1)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			s := string(data)
			if s == "2" { // Engine.IO ping -> pong
				_ = conn.WriteMessage(websocket.TextMessage, []byte("3"))
				continue
			}
			if !strings.HasPrefix(s, "42") {
				continue
			}
			// Skip the server-version announcement so we only assert on the
			// event we explicitly emitted below.
			var ev []json.RawMessage
			if json.Unmarshal([]byte(s[2:]), &ev) == nil && len(ev) >= 1 {
				var name string
				if json.Unmarshal(ev[0], &name) == nil && name == "on_server_version" {
					continue
				}
			}
			got <- s
			return
		}
	}()

	go app.emit("asset.create", map[string]any{"id": "xyz"})

	select {
	case pkt := <-got:
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(pkt[2:]), &arr); err != nil {
			t.Fatalf("event json: %v", err)
		}
		if len(arr) < 2 {
			t.Fatalf("event packet expected [name, payload], got %s", pkt)
		}
		var name string
		if err := json.Unmarshal(arr[0], &name); err != nil {
			t.Fatalf("event name: %v", err)
		}
		if name != "on_upload_success" {
			t.Fatalf("expected on_upload_success, got %q", name)
		}
		var payload map[string]any
		_ = json.Unmarshal(arr[1], &payload)
		if payload["id"] != "xyz" {
			t.Fatalf("payload id mismatch: %v", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for socket.io event")
	}
}

// TestSocketIOPollingHandshake validates the polling transport returns a
// valid Engine.IO open packet with a sid so a client can proceed to upgrade.
func TestSocketIOPollingHandshake(t *testing.T) {
	_, r, token := newTestServer(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/socket.io/?EIO=4&transport=polling", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("polling handshake: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 || body[0] != '0' {
		t.Fatalf("expected open packet, got %q", string(body))
	}
	// Body is "0{...}\n"; strip the '0' and the optional trailing newline.
	payload := bytes.TrimRight(body[1:], "\n")
	var open struct {
		Sid string `json:"sid"`
	}
	if err := json.Unmarshal(payload, &open); err != nil {
		t.Fatalf("open json: %v (body=%q)", err, string(body))
	}
	if open.Sid == "" {
		t.Fatal("polling open packet missing sid")
	}
}
