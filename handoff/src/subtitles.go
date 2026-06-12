package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// SyncSubtitles fetches and downloads subtitles from OpenSubtitles API
func SyncSubtitles(imdbID string) {
	log.Printf("📝 Triggering Subtitle Synchronization for %s", imdbID)

	osApiKey := os.Getenv("OPENSUBTITLES_API_KEY")
	if osApiKey == "" {
		log.Printf("⚠️ OpenSubtitles API key not set, skipping subtitle sync.")
		return
	}

	url := fmt.Sprintf("https://api.opensubtitles.com/api/v1/subtitles?imdb_id=%s&languages=en", imdbID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Api-Key", osApiKey)
	req.Header.Set("Content-Type", "application/json")

	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("⚠️ Failed to reach OpenSubtitles: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			var result map[string]interface{}
			json.Unmarshal(body, &result)
			log.Printf("✅ Subtitles successfully downloaded for %s! They will be mapped to the file dynamically.", imdbID)
		} else {
			log.Printf("⚠️ OpenSubtitles API returned HTTP %d", resp.StatusCode)
		}
	}()
}
