package main

import (
	"context"
	"encoding/json"
	"fmt"
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

	db.QueryRow("SELECT COUNT(*) FROM stream_urls").Scan(&totalURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE is_valid = 1").Scan(&validURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE fail_count > 0").Scan(&failedURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_cache").Scan(&cachedRequests)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE last_validated > ?", time.Now().Add(-10*time.Minute)).Scan(&recentValidations)

	w.Write([]byte(fmt.Sprintf(`{
		"totalStreams": %d,
		"validStreams": %d,
		"failedStreams": %d,
		"cachedRequests": %d,
		"recentValidations": %d,
		"uptime": "%s"
	}`,
		totalURLs, validURLs, failedURLs, cachedRequests, recentValidations,
		time.Since(startTime).Round(time.Second).String(),
	)))
}

func serveAPIBandwidth(w http.ResponseWriter, r *http.Request) {
	stats := GetBandwidthStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
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
		w.Write([]byte(fmt.Sprintf(`{"keys": %s}`, marshalJSON(keys))))
		return
	}

	if r.Method == "POST" {
		var req struct {
			Token string `json:"token"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		trimmed := strings.TrimSpace(req.Token)
		if trimmed != "" {
			// Check if already exists
			for _, existing := range rd.Keys {
				if existing.Token == trimmed {
					w.Write([]byte(`{"status": "ok", "message": "Key already exists"}`))
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
		w.Write([]byte(`{"status": "ok", "message": "Key added"}`))
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

		w.Write([]byte(`{"status": "ok", "message": "Key deleted"}`))
		return
	}
}

func serveAPISourcesGet(w http.ResponseWriter, r *http.Request) {
	sourcesMu.Lock()
	sources := make([]AddonSource, len(addonSources))
	copy(sources, addonSources)
	sourcesMu.Unlock()

	w.Write([]byte(fmt.Sprintf(`{"sources": %s}`, marshalJSON(sources))))
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

	w.Write([]byte(`{"status": "ok", "message": "Sources updated"}`))
}

func serveAPIMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var req struct {
			AddonURL string `json:"addon_url"`
			Alias    string `json:"alias"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		
		addonURL := strings.TrimSpace(req.AddonURL)
		if addonURL == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "addon_url is required"})
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
		json.NewEncoder(w).Encode(map[string]string{"alias": alias, "addon_url": addonURL})
		return
	}

	if r.Method == "DELETE" {
		r.ParseForm()
		alias := r.FormValue("alias")
		if alias != "" {
			aliasMappings.Delete(alias)
			saveMappings()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing alias"})
		return
	}

	mappings := make(map[string]Config)
	aliasMappings.Range(func(key, value interface{}) bool {
		mappings[key.(string)] = value.(Config)
		return true
	})

	w.Write([]byte(fmt.Sprintf(`{"mappings": %s}`, marshalJSON(mappings))))
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

	w.Write([]byte(`{"status": "ok", "message": "Cleaned failed and stale streams"}`))
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

	w.Write([]byte(`{"status": "ok", "message": "Cache cleared"}`))
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
		w.Write([]byte(fmt.Sprintf(`{"authenticated":%v,"auth_key_prefix":%q}`,
			key != "", prefix)))

	case "POST":
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid request body"}`))
			return
		}
		req.Email = strings.TrimSpace(req.Email)
		if req.Email == "" || req.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"email and password are required"}`))
			return
		}

		authKey, err := stremioLogin(req.Email, req.Password)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(fmt.Sprintf(`{"error":%q}`, err.Error())))
			return
		}

		stremioAuthMu.Lock()
		stremioAuthKey = authKey
		stremioAuthMu.Unlock()
		saveStremioAuth(authKey)
		w.Write([]byte(`{"status":"ok","message":"Authenticated"}`))

	case "DELETE":
		stremioAuthMu.Lock()
		stremioAuthKey = ""
		stremioAuthMu.Unlock()
		os.Remove(stremioAuthPath)
		w.Write([]byte(`{"status":"ok","message":"Disconnected"}`))

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
		w.Write(data)

	case "POST":
		var req struct {
			Limit int  `json:"limit"`
			Force bool `json:"force"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		stremioAuthMu.RLock()
		authKey := stremioAuthKey
		stremioAuthMu.RUnlock()

		if authKey == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"not authenticated with Stremio"}`))
			return
		}

		prefetchStatusMu.RLock()
		running := prefetchStatus.Running
		prefetchStatusMu.RUnlock()

		if running {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"job already running"}`))
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
		w.Write([]byte(`{"status":"started"}`))

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
	w.Write([]byte(`{"status":"ok"}`))
}
