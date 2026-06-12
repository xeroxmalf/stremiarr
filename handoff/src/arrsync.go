package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// SyncArrStack notifies Radarr/Sonarr that a new torrent was added to Real-Debrid and cached.
// It uses the generic "DownloadedMoviesScan" or "DownloadedEpisodesScan" commands.
func SyncArrStack(downloadPath string) {
	radarrURL := os.Getenv("RADARR_URL")
	radarrAPIKey := os.Getenv("RADARR_API_KEY")

	if radarrURL == "" || radarrAPIKey == "" {
		return
	}

	log.Printf("📡 Running Radarr bidirectional synchronization for path: %s", downloadPath)

	payload := map[string]interface{}{
		"name": "DownloadedMoviesScan",
		"path": downloadPath,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", radarrURL+"/api/v3/command", bytes.NewBuffer(body))
	req.Header.Set("X-Api-Key", radarrAPIKey)
	req.Header.Set("Content-Type", "application/json")

	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("⚠️ Failed to sync with Radarr: %v", err)
			return
		}
		defer resp.Body.Close()
		log.Printf("✅ Successfully triggered Radarr import scan")
	}()
}
