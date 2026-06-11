package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketConnection(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWebSocket(w, r)
	}))
	defer s.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("could not open a ws connection on %s %v", wsURL, err)
	}
	defer ws.Close()

	// Wait briefly for connection registration
	time.Sleep(100 * time.Millisecond)

	// Since broadcastStats depends on db which might be nil in this test,
	// let's manually write to all connected clients instead
	msg := map[string]interface{}{
		"type":    "test",
		"message": "hello world",
	}

	hub.mu.Lock()
	for conn := range hub.clients {
		conn.WriteJSON(msg)
	}
	hub.mu.Unlock()

	// Read messages from websocket
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 2; i++ {
		_, p, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("could not read message: %v", err)
		}

		var receivedMsg map[string]interface{}
		err = json.Unmarshal(p, &receivedMsg)
		if err != nil {
			t.Fatalf("failed to parse message: %v", err)
		}

		if receivedMsg["type"] == "test" {
			if receivedMsg["message"] != "hello world" {
				t.Errorf("received incorrect test message: %v", receivedMsg)
			}
			return
		}
	}
	t.Errorf("did not receive test message")
}
