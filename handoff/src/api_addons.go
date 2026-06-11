package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

const sourcesPath = "/data/sources.json"

// loadSourcesFromDisk loads addon sources from /data/sources.json.
// If the file does not exist, the hardcoded defaults in config.go remain.
func loadSourcesFromDisk() {
	if _, err := os.Stat(sourcesPath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(sourcesPath)
	if err != nil {
		log.Printf("⚠️ Failed to read sources.json: %v", err)
		return
	}

	var sources []AddonSource
	if err := json.Unmarshal(data, &sources); err != nil {
		log.Printf("⚠️ Failed to parse sources.json: %v", err)
		return
	}

	sourcesMu.Lock()
	addonSources = sources
	sourcesMu.Unlock()
	log.Printf("📁 Loaded %d addon sources from disk", len(sources))
}

// saveSourcesToDisk persists the current addonSources slice to /data/sources.json.
func saveSourcesToDisk() {
	sourcesMu.Lock()
	sources := make([]AddonSource, len(addonSources))
	copy(sources, addonSources)
	sourcesMu.Unlock()

	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		log.Printf("❌ Failed to serialize sources: %v", err)
		return
	}

	os.MkdirAll("/data", 0755)

	if err := os.WriteFile(sourcesPath, data, 0644); err != nil {
		log.Printf("❌ Failed to write sources.json: %v", err)
	}
}

// initAddonSources loads saved sources from disk, overriding the hardcoded defaults.
func initAddonSources() {
	loadSourcesFromDisk()
}

// fetchAndValidateManifest fetches the manifest at the given URL and validates it.
// Returns the parsed manifest map on success, or an error.
func fetchAndValidateManifest(rawURL string) (map[string]interface{}, error) {
	resp, err := httpClient.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.New("manifest fetch returned HTTP " + http.StatusText(resp.StatusCode) + ": " + string(body))
	}

	var manifest map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, errors.New("invalid manifest JSON: " + err.Error())
	}

	// Validate required fields
	if _, ok := manifest["id"].(string); !ok {
		return nil, errors.New("manifest missing required field: id")
	}
	if _, ok := manifest["name"].(string); !ok {
		return nil, errors.New("manifest missing required field: name")
	}
	if _, ok := manifest["version"].(string); !ok {
		return nil, errors.New("manifest missing required field: version")
	}

	// Validate resources array contains "stream"
	resources, ok := manifest["resources"]
	if !ok {
		return nil, errors.New("manifest missing required field: resources")
	}
	resourcesSlice, ok := resources.([]interface{})
	if !ok {
		return nil, errors.New("manifest resources is not an array")
	}
	hasStream := false
	for _, r := range resourcesSlice {
		// resources can be strings or objects with "name" field
		switch v := r.(type) {
		case string:
			if v == "stream" {
				hasStream = true
			}
		case map[string]interface{}:
			if v["name"] == "stream" {
				hasStream = true
			}
		}
	}
	if !hasStream {
		return nil, errors.New("manifest resources does not include \"stream\"")
	}

	return manifest, nil
}

// serveAPIAddons handles POST/DELETE /api/addons/discover
func serveAPIAddons(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if strings.HasPrefix(path, "/api/addons/discover") {
		switch r.Method {
		case "POST":
			discoverAddon(w, r)
		case "DELETE":
			removeDiscoveredAddon(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.Error(w, "Not found", http.StatusNotFound)
}

func discoverAddon(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
		return
	}

	log.Printf("🔍 [Discover] Fetching manifest from: %s", rawURL)

	manifest, err := fetchAndValidateManifest(rawURL)
	if err != nil {
		log.Printf("❌ [Discover] Validation failed for %s: %v", rawURL, err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	id := manifest["id"].(string)
	name := manifest["name"].(string)

	// Check if this URL is already in the sources list
	sourcesMu.Lock()
	for _, src := range addonSources {
		if src.URL == rawURL {
			sourcesMu.Unlock()
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "addon source already exists", "name": name, "id": id})
			return
		}
	}
	addonSources = append(addonSources, AddonSource{
		Name:    name,
		URL:     rawURL,
		Enabled: true,
	})
	sourcesMu.Unlock()

	saveSourcesToDisk()

	log.Printf("✅ [Discover] Added addon source: %s (%s)", name, id)

	// Build resources list for response
	resourcesRaw, _ := manifest["resources"].([]interface{})
	var resourceNames []string
	for _, r := range resourcesRaw {
		switch v := r.(type) {
		case string:
			resourceNames = append(resourceNames, v)
		case map[string]interface{}:
			if n, ok := v["name"].(string); ok {
				resourceNames = append(resourceNames, n)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"name":      name,
		"id":        id,
		"resources": resourceNames,
	})
}

func removeDiscoveredAddon(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		http.Error(w, `{"error":"url query parameter is required"}`, http.StatusBadRequest)
		return
	}

	sourcesMu.Lock()
	var newSources []AddonSource
	found := false
	for _, src := range addonSources {
		if src.URL == rawURL {
			found = true
			continue
		}
		newSources = append(newSources, src)
	}
	addonSources = newSources
	sourcesMu.Unlock()

	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "addon source not found"})
		return
	}

	saveSourcesToDisk()
	log.Printf("🗑️ [Discover] Removed addon source: %s", rawURL)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Addon source removed"})
}
