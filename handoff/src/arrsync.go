package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// SyncArrStack notifies Radarr/Sonarr that a new torrent was added to Real-Debrid and cached.
// It uses the generic "DownloadedMoviesScan" or "DownloadedEpisodesScan" commands.
func SyncArrStack(downloadPath string) {
	radarrURL := os.Getenv("RADARR_URL")
	radarrAPIKey := os.Getenv("RADARR_API_KEY")
	sonarrURL := os.Getenv("SONARR_URL")
	sonarrAPIKey := os.Getenv("SONARR_API_KEY")

	if radarrURL == "" && sonarrURL == "" {
		return
	}

	go func() {
		if radarrURL != "" && radarrAPIKey != "" {
			syncWithArr("Radarr", strings.TrimRight(radarrURL, "/")+"/api/v3/command", radarrAPIKey, "DownloadedMoviesScan", downloadPath)
		}

		if sonarrURL != "" && sonarrAPIKey != "" {
			syncWithArr("Sonarr", strings.TrimRight(sonarrURL, "/")+"/api/v3/command", sonarrAPIKey, "DownloadedEpisodesScan", downloadPath)
		}
	}()
}

func syncWithArr(name, apiURL, apiKey, command, path string) {
	log.Printf("📡 Running %s synchronization for path: %s", name, path)

	payload := map[string]interface{}{
		"name": command,
		"path": path,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("⚠️ Failed to marshal %s payload: %v", name, err)
		return
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("⚠️ Failed to create %s request: %v", name, err)
		return
	}
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("⚠️ Failed to sync with %s: %v", name, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 202 {
		log.Printf("✅ Successfully triggered %s import scan", name)
	} else {
		log.Printf("⚠️ %s returned HTTP %d", name, resp.StatusCode)
	}
}
