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

func (h *WSHub) getStats() map[string]interface{} {
	var totalURLs, validURLs, failedURLs int
	var cachedRequests, recentValidations int

	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls").Scan(&totalURLs); err != nil {
		log.Printf("⚠️ ws stats error: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE is_valid = TRUE").Scan(&validURLs); err != nil {
		log.Printf("⚠️ ws stats error: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE fail_count > 0").Scan(&failedURLs); err != nil {
		log.Printf("⚠️ ws stats error: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_cache").Scan(&cachedRequests); err != nil {
		log.Printf("⚠️ ws stats error: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE last_validated > ?", time.Now().Add(-10*time.Minute)).Scan(&recentValidations); err != nil {
		log.Printf("⚠️ ws stats error: %v", err)
	}

	return map[string]interface{}{
		"type":              "stats",
		"totalStreams":      totalURLs,
		"validStreams":      validURLs,
		"failedStreams":     failedURLs,
		"cachedRequests":    cachedRequests,
		"recentValidations": recentValidations,
		"uptime":            time.Since(startTime).Round(time.Second).String(),
	}
}

func (h *WSHub) broadcastStats() {
	h.mu.Lock()
	if len(h.clients) == 0 {
		h.mu.Unlock()
		return
	}
	// Snapshot clients to avoid holding lock during DB queries + writes
	snapshot := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		snapshot = append(snapshot, c)
	}
	h.mu.Unlock()

	stats := h.getStats()

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, conn := range snapshot {
		err := conn.WriteJSON(stats)
		if err != nil {
			conn.Close()
			delete(h.clients, conn)
			activeConnections.Dec()
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
	if err := conn.WriteJSON(hub.getStats()); err != nil {
		log.Printf("[WS] Failed to send initial stats: %v", err)
		hub.removeClient(conn)
		return
	}

	// Keep connection alive until client drops
	go func() {
		defer hub.removeClient(conn)
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			log.Printf("[WS] SetReadDeadline failed: %v", err)
			return
		}
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
			// Ping-pong: extend deadline on each message
			if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
				log.Printf("[WS] SetReadDeadline failed: %v", err)
				break
			}
		}
	}()
}
