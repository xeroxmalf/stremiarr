package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func serveAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path
	switch {
	case path == "/api/stats":
		RequireRole("admin", serveAPIStats)(w, r)
	case path == "/api/bandwidth":
		RequireRole("admin", serveAPIBandwidth)(w, r)
	case path == "/api/ws":
		serveWebSocket(w, r)
	case path == "/api/keys":
		RequireRole("admin", serveAPIKeys)(w, r)
	case path == "/api/sources" && r.Method == "GET":
		RequireRole("admin", serveAPISourcesGet)(w, r)
	case path == "/api/sources" && r.Method == "PUT":
		RequireRole("admin", serveAPISourcesUpdate)(w, r)
	case path == "/api/mappings":
		RequireRole("admin", serveAPIMappings)(w, r)
	case path == "/api/clean":
		RequireRole("admin", serveAPIClean)(w, r)
	case path == "/api/cache":
		serveAPICache(w, r)
	case path == "/transcode":
		streamURL := r.URL.Query().Get("stream")
		if streamURL != "" {
			TranscodeAudio(w, r, streamURL)
		} else {
			http.Error(w, "Missing stream URL", http.StatusBadRequest)
		}
	case strings.HasPrefix(path, "/api/addons"):
		serveAPIAddons(w, r)
	case path == "/api/prefetch":
		serveAPIPrefetch(w, r)
	case path == "/api/prefetch/stop":
		serveAPIPrefetchStop(w, r)
	case path == "/api/stremio/auth":
		serveAPIStremioAuth(w, r)
	case path == "/auth/login":
		handleOAuthLogin(w, r)
	case path == "/auth/callback":
		handleOAuthCallback(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func serveAPIStats(w http.ResponseWriter, r *http.Request) {
	var totalURLs, validURLs, failedURLs int
	var cachedRequests, recentValidations int

	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls").Scan(&totalURLs); err != nil {
		log.Printf("⚠️ db error: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE is_valid = TRUE").Scan(&validURLs); err != nil {
		log.Printf("⚠️ db error: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE fail_count > 0").Scan(&failedURLs); err != nil {
		log.Printf("⚠️ db error: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_cache").Scan(&cachedRequests); err != nil {
		log.Printf("⚠️ db error: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE last_validated > ?", time.Now().Add(-10*time.Minute)).Scan(&recentValidations); err != nil {
		log.Printf("⚠️ db error: %v", err)
	}

	if _, err := w.Write([]byte(fmt.Sprintf(`{
		"totalStreams": %d,
		"validStreams": %d,
		"failedStreams": %d,
		"cachedRequests": %d,
		"recentValidations": %d,
		"uptime": "%s"
	}`,
		totalURLs, validURLs, failedURLs, cachedRequests, recentValidations,
		time.Since(startTime).Round(time.Second).String(),
	))); err != nil {
		log.Printf("⚠️ Failed to write stats response: %v", err)
	}
}

func serveAPIBandwidth(w http.ResponseWriter, r *http.Request) {
	stats := GetBandwidthStats()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("⚠️ Failed to encode bandwidth stats: %v", err)
	}
}

func serveAPIKeys(w http.ResponseWriter, r *http.Request) {
	var rd *RealDebridProvider
	for _, p := range debridProviders {
		if provider, ok := p.(*RealDebridProvider); ok {
			rd = provider
			break
		}
	}
	if rd == nil {
		rd = &RealDebridProvider{}
		debridProviders = append(debridProviders, rd)
	}

	rd.Mu.Lock()
	defer rd.Mu.Unlock()

	if r.Method == "GET" {
		type KeyInfo struct {
			Token      string `json:"token"`
			Locked     bool   `json:"locked"`
			LockedSecs int    `json:"locked_secs"`
		}
		var keys []KeyInfo
		now := time.Now()
		for _, k := range rd.Keys {
			display := k.Token
			if len(display) > 8 {
				display = display[:4] + "..." + display[len(display)-4:]
			}
			info := KeyInfo{Token: display, Locked: k.LockedOutUntil.After(now)}
			if info.Locked {
				info.LockedSecs = int(k.LockedOutUntil.Sub(now).Seconds())
			}
			keys = append(keys, info)
		}
		if _, err := w.Write([]byte(fmt.Sprintf(`{"keys": %s}`, marshalJSON(keys)))); err != nil {
			log.Printf("⚠️ Failed to write keys response: %v", err)
		}
		return
	}

	if r.Method == "POST" {
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		trimmed := strings.TrimSpace(req.Token)
		if trimmed != "" {
			// Check if already exists
			for _, existing := range rd.Keys {
				if existing.Token == trimmed {
					if _, err := w.Write([]byte(`{"status": "ok", "message": "Key already exists"}`)); err != nil {
						log.Printf("⚠️ Failed to write response: %v", err)
					}
					return
				}
			}
			rd.Keys = append(rd.Keys, &RdKey{Token: trimmed})

			// Save to disk
			var allDiskKeys []string
			for _, k := range rd.Keys {
				// Don't save env keys to disk
				envKeys := os.Getenv("RD_API_KEY")
				if !strings.Contains(envKeys, k.Token) {
					allDiskKeys = append(allDiskKeys, k.Token)
				}
			}
			saveKeysToDisk(allDiskKeys)
		}
		if _, err := w.Write([]byte(`{"status": "ok", "message": "Key added"}`)); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}
		return
	}

	if r.Method == "DELETE" {
		tokenToDel := r.URL.Query().Get("token")
		// Cannot delete from env
		if strings.Contains(os.Getenv("RD_API_KEY"), tokenToDel) {
			http.Error(w, "Cannot delete key provided via ENV", http.StatusBadRequest)
			return
		}

		var newKeys []*RdKey
		for _, k := range rd.Keys {
			display := k.Token
			if len(display) > 8 {
				display = display[:4] + "..." + display[len(display)-4:]
			}
			if display != tokenToDel && k.Token != tokenToDel {
				newKeys = append(newKeys, k)
			}
		}
		rd.Keys = newKeys

		var allDiskKeys []string
		for _, k := range rd.Keys {
			if !strings.Contains(os.Getenv("RD_API_KEY"), k.Token) {
				allDiskKeys = append(allDiskKeys, k.Token)
			}
		}
		saveKeysToDisk(allDiskKeys)
		if _, err := w.Write([]byte(`{"status": "ok", "message": "Key deleted"}`)); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}
		return
	}
}

func serveAPISourcesGet(w http.ResponseWriter, r *http.Request) {
	sourcesMu.Lock()
	sources := make([]AddonSource, len(addonSources))
	copy(sources, addonSources)
	sourcesMu.Unlock()

	if _, err := w.Write([]byte(fmt.Sprintf(`{"sources": %s}`, marshalJSON(sources)))); err != nil {
		log.Printf("⚠️ Failed to write sources: %v", err)
	}
}

func serveAPISourcesUpdate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		Sources []AddonSource `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	sourcesMu.Lock()
	addonSources = req.Sources
	sourcesMu.Unlock()
	saveSourcesToDisk()

	if _, err := w.Write([]byte(`{"status": "ok", "message": "Sources updated"}`)); err != nil {
		log.Printf("⚠️ Failed to write response: %v", err)
	}
}

func serveAPIMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var req struct {
			AddonURL string `json:"addon_url"`
			Alias    string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		addonURL := strings.TrimSpace(req.AddonURL)
		if addonURL == "" {
			w.WriteHeader(http.StatusBadRequest)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": "addon_url is required"}); err != nil {
				log.Printf("⚠️ Failed to write error response: %v", err)
			}
			return
		}

		alias := strings.TrimSpace(req.Alias)
		if alias == "" {
			alias = generateID()
		}

		conf := Config{AddonURL: addonURL}
		aliasMappings.Store(alias, conf)
		saveMappings()

		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]string{"alias": alias, "addon_url": addonURL}); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}
		return
	}

	if r.Method == "DELETE" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		alias := r.FormValue("alias")
		if alias != "" {
			aliasMappings.Delete(alias)
			saveMappings()
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
				log.Printf("⚠️ Failed to write response: %v", err)
			}
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "missing alias"}); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}
		return
	}

	mappings := make(map[string]Config)
	aliasMappings.Range(func(key, value interface{}) bool {
		mappings[key.(string)] = value.(Config)
		return true
	})

	if _, err := w.Write([]byte(fmt.Sprintf(`{"mappings": %s}`, marshalJSON(mappings)))); err != nil {
		log.Printf("⚠️ Failed to write response: %v", err)
	}
}

func serveAPIClean(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Delete streams with 3+ failures (permanently broken)
	_, err := db.Exec("DELETE FROM stream_urls WHERE fail_count >= 3")
	if err != nil {
		http.Error(w, "Failed to clean streams", http.StatusInternalServerError)
		return
	}

	// Reset streams with 1-2 failures so they get re-validated (not counted as failed)
	_, err = db.Exec("UPDATE stream_urls SET fail_count = 0 WHERE fail_count > 0")
	if err != nil {
		http.Error(w, "Failed to reset stream failures", http.StatusInternalServerError)
		return
	}

	// Mark stale streams as invalid so they get re-validated
	_, err = db.Exec("UPDATE stream_urls SET is_valid = FALSE WHERE last_validated < ?", time.Now().Add(-2*time.Hour))
	if err != nil {
		http.Error(w, "Failed to update invalid streams", http.StatusInternalServerError)
		return
	}

	if _, err = w.Write([]byte(`{"status": "ok", "message": "Cleaned failed and stale streams"}`)); err != nil {
		log.Printf("⚠️ Failed to write response: %v", err)
	}
}

func serveAPICache(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear all cached streams
	_, err := db.Exec("DELETE FROM stream_cache")
	if err != nil {
		http.Error(w, "Failed to clear cache", http.StatusInternalServerError)
		return
	}

	// Clear in-memory cache
	catalogCache.Clear()

	if _, err = w.Write([]byte(`{"status": "ok", "message": "Cache cleared"}`)); err != nil {
		log.Printf("⚠️ Failed to write response: %v", err)
	}
}

func marshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// serveAPIStremioAuth handles GET/POST/DELETE /api/stremio/auth
func serveAPIStremioAuth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		stremioAuthMu.RLock()
		key := stremioAuthKey
		stremioAuthMu.RUnlock()

		prefix := ""
		if len(key) >= 8 {
			prefix = key[:8]
		} else if len(key) > 0 {
			prefix = key
		}
		if _, err := w.Write([]byte(fmt.Sprintf(`{"authenticated":%v,"auth_key_prefix":%q}`,
			key != "", prefix))); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}

	case "POST":
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			if _, wErr := w.Write([]byte(`{"error":"invalid request body"}`)); wErr != nil {
				log.Printf("⚠️ Failed to write error response: %v", wErr)
			}
			return
		}
		req.Email = strings.TrimSpace(req.Email)
		if req.Email == "" || req.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte(`{"error":"email and password are required"}`)); err != nil {
				log.Printf("⚠️ Failed to write error response: %v", err)
			}
			return
		}

		authKey, err := stremioLogin(req.Email, req.Password)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			if _, err := w.Write([]byte(fmt.Sprintf(`{"error":%q}`, err.Error()))); err != nil {
				log.Printf("⚠️ Failed to write error response: %v", err)
			}
			return
		}

		stremioAuthMu.Lock()
		stremioAuthKey = authKey
		stremioAuthMu.Unlock()
		saveStremioAuth(authKey)
		if _, err := w.Write([]byte(`{"status":"ok","message":"Authenticated"}`)); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}

	case "DELETE":
		stremioAuthMu.Lock()
		stremioAuthKey = ""
		stremioAuthMu.Unlock()
		os.Remove(getStremioAuthPath())
		if _, err := w.Write([]byte(`{"status":"ok","message":"Disconnected"}`)); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveAPIPrefetch handles GET/POST /api/prefetch
func serveAPIPrefetch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		prefetchStatusMu.RLock()
		data, _ := json.Marshal(prefetchStatus)
		prefetchStatusMu.RUnlock()
		if _, err := w.Write(data); err != nil {
			log.Printf("⚠️ Failed to write prefetch status: %v", err)
		}

	case "POST":
		var req struct {
			Limit int  `json:"limit"`
			Force bool `json:"force"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		stremioAuthMu.RLock()
		authKey := stremioAuthKey
		stremioAuthMu.RUnlock()

		if authKey == "" {
			w.WriteHeader(http.StatusUnauthorized)
			if _, err := w.Write([]byte(`{"error":"not authenticated with Stremio"}`)); err != nil {
				log.Printf("⚠️ Failed to write response: %v", err)
			}
			return
		}

		prefetchStatusMu.RLock()
		running := prefetchStatus.Running
		prefetchStatusMu.RUnlock()

		if running {
			w.WriteHeader(http.StatusConflict)
			if _, err := w.Write([]byte(`{"error":"job already running"}`)); err != nil {
				log.Printf("⚠️ Failed to write response: %v", err)
			}
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		prefetchCancelMu.Lock()
		if prefetchCancel != nil {
			prefetchCancel()
		}
		prefetchCancel = cancel
		prefetchCancelMu.Unlock()

		prefetchStatusMu.Lock()
		*prefetchStatus = PrefetchStatus{
			Running:   true,
			StartedAt: time.Now(),
			Log:       []string{},
		}
		prefetchStatusMu.Unlock()

		go runPrefetchJob(ctx, authKey, req.Limit, req.Force)
		if _, err := w.Write([]byte(`{"status":"started"}`)); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveAPIPrefetchStop handles POST /api/prefetch/stop
func serveAPIPrefetchStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prefetchCancelMu.Lock()
	if prefetchCancel != nil {
		prefetchCancel()
		prefetchCancel = nil
	}
	prefetchCancelMu.Unlock()
	if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
		log.Printf("⚠️ Failed to write response: %v", err)
	}
}
