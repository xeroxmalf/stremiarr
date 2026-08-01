package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// observeRequestDuration wraps handlers to record latency
func observeRequestDuration(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: 200}
		next(rw, r)
		// Normalize path: /api/* -> /api/{action}
		path := normalizePath(r.URL.Path)
		ObserveRequestDuration(path, r.Method, sanitizeStatusCode(rw.statusCode), time.Since(start))
	}
}

func normalizePath(path string) string {
	// Collapse segments after prefix for cardinality control
	if strings.HasPrefix(path, "/api/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			return "/api/" + parts[2]
		}
		return path
	}
	// Stream/play: /{id}/{action}/...
	parts := strings.Split(path, "/")
	if len(parts) >= 3 {
		return "/" + parts[1] + "/" + parts[2]
	}
	return path
}

func sanitizeStatusCode(code int) string {
	// Bucket status codes to control label cardinality
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

// SWR refresh control: avoid redundant concurrent refreshes of the same URL
var swrMu sync.Mutex
var swrInFlight = make(map[string]bool)

func routeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Public health endpoint for Docker healthchecks (no auth required)
	if r.URL.Path == "/health" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("⚠️ Failed to write health response: %v", err)
		}
		return
	}

	if r.URL.Path == "/metrics" {
		promhttp.Handler().ServeHTTP(w, r)
		return
	}

	// Web UI
	if r.URL.Path == "/" || r.URL.Path == "/ui" || r.URL.Path == "/ui/" || r.URL.Path == "/dashboard" {
		// If admin password is set, require auth for UI
		if AdminPassword != "" && r.URL.Path != "/login" {
			cookie, err := r.Cookie("handoff_session")
			if err != nil || !validateSession(cookie.Value) {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
		}
		serveWebUI(w, r)
		return
	}

	// Login page
	if r.URL.Path == "/login" {
		serveLoginPage(w, r)
		return
	}

	// API and auth endpoints
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
		serveAPI(w, r)
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	// Legacy route
	if path == "generate" || path == "ui" {
		serveWebUI(w, r)
		return
	}

	// Need at least one segment
	if len(parts) == 0 {
		serveWebUI(w, r)
		return
	}

	idOrConfig := parts[0]
	// Look up mapped config for standard aliases
	mappedConfInt, exists := aliasMappings.Load(idOrConfig)
	if exists {
		mappedConf := mappedConfInt.(Config)
		subPath := strings.Join(parts[1:], "/")
		rawQuery := r.URL.RawQuery
		if subPath == "manifest.json" {
			manifestHandler(w, r, mappedConf, "manifest.json", rawQuery)
			return
		}
		if strings.HasPrefix(subPath, "catalog/") || strings.HasPrefix(subPath, "meta/") {
			proxyRequest(w, r, mappedConf.AddonURL, subPath, rawQuery)
			return
		}
		if strings.HasPrefix(subPath, "stream/") {
			metricStreamsRequested.WithLabelValues(idOrConfig).Inc()
			streamHandler(w, r, mappedConf, idOrConfig, subPath, rawQuery)
			return
		}
		if strings.HasPrefix(subPath, "play") {
			playHandler(w, r, mappedConf)
			return
		}
		// Unknown path under alias (e.g., /alias/login) — redirect to root UI/login
		if strings.HasPrefix(subPath, "login") || strings.HasPrefix(subPath, "ui") {
			http.Redirect(w, r, "/"+subPath, http.StatusFound)
			return
		}
	}

	// Base64-encoded config path
	var conf Config
	configData, err := base64.RawURLEncoding.DecodeString(idOrConfig)
	if err != nil {
		http.Error(w, "Invalid Configuration format or Unknown Alias", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(configData, &conf); err != nil {
		http.Error(w, "Failed to parse configuration", http.StatusBadRequest)
		return
	}

	if len(parts) < 2 {
		http.Error(w, "Missing action in request path", http.StatusBadRequest)
		return
	}

	action := parts[1]
	switch action {
	case "play":
		playHandler(w, r, conf)
	case "stream":
		metricStreamsRequested.WithLabelValues("raw_config").Inc()
		streamHandler(w, r, conf, idOrConfig, strings.Join(parts[1:], "/"), r.URL.RawQuery)
	case "manifest.json":
		manifestHandler(w, r, conf, strings.Join(parts[1:], "/"), r.URL.RawQuery)
	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

// --------------------------------
// ENHANCED STREAM ROUTER (SWR CACHING)
// --------------------------------
func streamHandler(w http.ResponseWriter, r *http.Request, conf Config, idOrConfig string, subPath string, rawQuery string) {
	targetURL := getTargetURL(conf.AddonURL, subPath, rawQuery)

	// Check SWR cache first
	cachedStreams, err := cache.GetStreamCache(targetURL)

	// 1. Optimistic SWR Serve
	if err == nil && cachedStreams != "" {
		RecordCacheHit()
		serveStreamsJSON(w, r, []byte(cachedStreams), idOrConfig)

		// Fire Stale-While-Revalidate worker into background (deduplicated)
		go maybeRefreshStreams(targetURL, subPath, rawQuery, conf)
		return
	}

	RecordCacheMiss()

	// 2. Cache Miss: sync fetch with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	body, err := fetchAndCacheStreams(ctx, targetURL, subPath, rawQuery, conf)
	if err != nil {
		// On timeout/failure, serve stale if available
		if cachedStreams != "" {
			RecordCacheHit()
			log.Printf("[Stream] ⚠️ Fetch failed, serving stale cache: %v", err)
			serveStreamsJSON(w, r, []byte(cachedStreams), idOrConfig)
			return
		}
		log.Printf("[Stream] ❌ Failed to fetch streams: %v", err)
		http.Error(w, "Failed to fetch streams", http.StatusBadGateway)
		return
	}

	serveStreamsJSON(w, r, body, idOrConfig)
}

// maybeRefreshStreams deduplicates concurrent SWR refreshes for the same URL
func maybeRefreshStreams(targetURL, subPath, rawQuery string, conf Config) {
	swrMu.Lock()
	if swrInFlight[targetURL] {
		swrMu.Unlock()
		return
	}
	swrInFlight[targetURL] = true
	swrMu.Unlock()

	defer func() {
		swrMu.Lock()
		delete(swrInFlight, targetURL)
		swrMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = fetchAndCacheStreams(ctx, targetURL, subPath, rawQuery, conf)
}

type StreamFetcher struct {
	Streams []interface{}
}

func manifestHandler(w http.ResponseWriter, r *http.Request, conf Config, subPath string, rawQuery string) {
	targetURL := getTargetURL(conf.AddonURL, subPath, rawQuery)

	resp, err := httpClient.Get(targetURL)
	if err != nil {
		log.Printf("[Manifest] ❌ Failed to fetch manifest: %v", err)
		http.Error(w, "Failed to fetch manifest", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		http.Error(w, "Manifest unavailable", http.StatusBadGateway)
		return
	}

	var manifest map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		log.Printf("[Manifest] ⚠️ Failed to decode manifest: %v", err)
		http.Error(w, "Invalid manifest", http.StatusBadGateway)
		return
	}

	// Preserve original for response, add Handoff branding
	if name, ok := manifest["name"].(string); ok {
		manifest["name"] = name + " (Handoff)"
	}
	if id, ok := manifest["id"].(string); ok {
		manifest["id"] = id + ".handoff"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(manifest); err != nil {
		log.Printf("⚠️ Failed to encode manifest: %v", err)
	}
}
