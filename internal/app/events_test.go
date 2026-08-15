package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebsocketSync(t *testing.T) {
	app, r, token := newTestServer(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/events"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial /api/events: %v", err)
	}
	defer ws.Close()

	// First frame is the init handshake.
	_, initMsg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var initEv map[string]any
	if err := json.Unmarshal(initMsg, &initEv); err != nil {
		t.Fatalf("init json: %v", err)
	}
	if initEv["type"] != "init" {
		t.Fatalf("expected init handshake, got %v", initEv)
	}

	// Publishing an event should stream it to the connected client.
	go app.emit("asset.create", map[string]any{"id": "abc"})

	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	var ev map[string]any
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("event json: %v", err)
	}
	if ev["type"] != "asset.create" {
		t.Fatalf("expected asset.create, got %v", ev)
	}
	p, _ := ev["payload"].(map[string]any)
	if p["id"] != "abc" {
		t.Fatalf("payload id mismatch: %v", p)
	}
}

func TestWebsocketRequiresAuth(t *testing.T) {
	_, r, _ := newTestServer(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/events"
	// No Authorization header -> the auth guard rejects with 401, so the
	// websocket handshake must fail (not upgrade to 101).
	_, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail without auth token")
	}
}
