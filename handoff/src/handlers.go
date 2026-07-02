package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func routeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Public health endpoint for Docker healthchecks (no auth required)
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}
		return
	}

	if r.URL.Path == "/dashboard" || r.URL.Path == "/" || r.URL.Path == "" {
		serveWebUI(w, r)
		return
	}

	if r.URL.Path == "/metrics" {
		promhttp.Handler().ServeHTTP(w, r)
		return
	}

	// Web UI and API endpoints (authenticated)
	if r.URL.Path == "/ui" || r.URL.Path == "/ui/" {
		serveWebUI(w, r)
		return
	}

	// API endpoints
	if strings.HasPrefix(r.URL.Path, "/api/") {
		serveAPI(w, r)
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	if path == "generate" || path == "ui" {
		serveWebUI(w, r)
		return
	}

	idOrConfig := parts[0]
	// Look up mapped config for standard aliases
	mappedConfInt, exists := aliasMappings.Load(idOrConfig)
	if exists {
		mappedConf := mappedConfInt.(Config)
		// Standard handling for valid alias
		subPath := strings.Join(parts[1:], "/")
		rawQuery := r.URL.RawQuery
		if subPath == "manifest.json" {
			manifestHandler(w, r, mappedConf, "manifest.json", "")
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
	}

	var conf Config
	configData, err := base64.RawURLEncoding.DecodeString(idOrConfig)
	if err != nil {
		log.Printf("[Router] ⚠️ Unknown alias or invalid config format requested: %s", idOrConfig)
		http.Error(w, "Invalid Configuration format or Unknown Alias", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(configData, &conf); err != nil {
		log.Printf("[Router] ⚠️ Failed to parse configuration from base64")
		http.Error(w, "Failed to parse configuration", http.StatusBadRequest)
		return
	}

	action := parts[1]
	if action == "play" {
		playHandler(w, r, conf)
	} else if action == "stream" {
		metricStreamsRequested.WithLabelValues("raw_config").Inc()
		streamHandler(w, r, conf, idOrConfig, strings.Join(parts[1:], "/"), r.URL.RawQuery)
	} else if action == "manifest.json" {
		manifestHandler(w, r, conf, strings.Join(parts[1:], "/"), r.URL.RawQuery)
	}
}

// --------------------------------
// ENHANCED STREAM ROUTER (SWR CACHING)
// --------------------------------
func streamHandler(w http.ResponseWriter, r *http.Request, conf Config, idOrConfig string, subPath string, rawQuery string) {
	targetURL := getTargetURL(conf.AddonURL, subPath, rawQuery)
	log.Printf("[Stream] 🔍 Requesting streams for: %s", targetURL)

	cachedStreams, err := cache.GetStreamCache(targetURL)

	// 1. Optimistic SWR Serve
	if err == nil && cachedStreams != "" {
		log.Printf("[Stream] ⚡ Serving Optimistic Cache for: %s", targetURL)
		serveStreamsJSON(w, r, []byte(cachedStreams), idOrConfig)

		// Fire Stale-While-Revalidate worker into background
		go func() {
			if _, err := fetchAndCacheStreams(targetURL, conf); err != nil {
				log.Printf("⚠️ SWR worker failed to fetch streams: %v", err)
			}
		}()
		return
	}

	// 2. Cache Miss Sync Fetch
	body, err := fetchAndCacheStreams(targetURL, conf)
	if err != nil {
		log.Printf("[Stream] ❌ Failed to fetch streams: %v", err)
		http.Error(w, "Failed to fetch streams", http.StatusBadGateway)
		return
	}

	serveStreamsJSON(w, r, body, idOrConfig)
}

type StreamFetcher struct {
	Streams []interface{}
}

func manifestHandler(w http.ResponseWriter, r *http.Request, conf Config, subPath string, rawQuery string) {
	targetURL := getTargetURL(conf.AddonURL, subPath, rawQuery)
	log.Printf("[Manifest] 📦 Intercepting manifest from: %s", conf.AddonURL)

	resp, err := httpClient.Get(targetURL)
	if err != nil {
		log.Printf("[Manifest] ❌ Failed to fetch manifest: %v", err)
		http.Error(w, "Failed to fetch manifest", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var manifest map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		log.Printf("[Meta] ⚠️ Failed to decode manifest: %v", err)
	}

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
	log.Printf("[Manifest] ✅ Successfully wrapped and injected manifest identity.")
}
