package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// In a real production app, restrict CheckOrigin
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSHub struct {
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
}

var hub = &WSHub{
	clients: make(map[*websocket.Conn]bool),
}

func initWebSocketHub() {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			hub.broadcastStats()
		}
	}()
}

func (h *WSHub) addClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = true
	activeConnections.Inc()
}

func (h *WSHub) removeClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
	conn.Close()
	activeConnections.Dec()
}

func (h *WSHub) broadcastStats() {
	h.mu.Lock()
	if len(h.clients) == 0 {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	var totalURLs, validURLs, failedURLs int
	var cachedRequests, recentValidations int

	db.QueryRow("SELECT COUNT(*) FROM stream_urls").Scan(&totalURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE is_valid = 1").Scan(&validURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE fail_count > 0").Scan(&failedURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_cache").Scan(&cachedRequests)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE last_validated > ?", time.Now().Add(-10*time.Minute)).Scan(&recentValidations)

	stats := map[string]interface{}{
		"type":              "stats",
		"totalStreams":      totalURLs,
		"validStreams":      validURLs,
		"failedStreams":     failedURLs,
		"cachedRequests":    cachedRequests,
		"recentValidations": recentValidations,
		"uptime":            time.Since(startTime).Round(time.Second).String(),
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		err := conn.WriteJSON(stats)
		if err != nil {
			log.Printf("[WS] Error writing to client: %v", err)
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

func serveWebSocket(w http.ResponseWriter, r *http.Request) {

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	hub.addClient(conn)

	// Send initial stats immediately
	hub.broadcastStats()

	// Keep connection alive until client drops
	go func() {
		defer hub.removeClient(conn)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
