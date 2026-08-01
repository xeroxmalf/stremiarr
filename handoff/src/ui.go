package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed ui.html
var uiTemplate string

//go:embed login.html
var loginTemplate string

func serveWebUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(uiTemplate)); err != nil {
		log.Printf("⚠️ Failed to write UI template: %v", err)
	}
}

func serveLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(loginTemplate)); err != nil {
		log.Printf("⚠️ Failed to write login template: %v", err)
	}
}

// serveAPIConfig returns current configuration state (read-only)
func serveAPIConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := map[string]interface{}{
		"port":               os.Getenv("PORT"),
		"database_type":      os.Getenv("DATABASE_TYPE"),
		"rclone_configured":  RcloneUrl != "",
		"rd_keys_count":      0,
		"admin_auth_enabled": AdminPassword != "",
		"rbac_enabled":       os.Getenv("RBAC_ENABLED") == "true",
		"uptime":             time.Since(startTime).Round(time.Second).String(),
		"version":            "1.0.0",
	}

	// Count RD keys
	providersMu.RLock()
	for _, p := range debridProviders {
		if rd, ok := p.(*RealDebridProvider); ok {
			rd.Mu.Lock()
			config["rd_keys_count"] = len(rd.Keys)
			rd.Mu.Unlock()
			break
		}
	}
	providersMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config); err != nil {
		log.Printf("⚠️ Failed to encode config: %v", err)
	}
}
