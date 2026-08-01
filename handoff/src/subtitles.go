package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	subtitleMu    sync.Mutex
	subtitleCache = make(map[string]subtitleCacheEntry)
	subtitleTTL   = 24 * time.Hour
	pendingFetch  = make(map[string]bool) // track in-flight fetches to prevent goroutine leaks
)

// subtitleCacheEntry tracks a cached subtitle download URL and its expiration
type subtitleCacheEntry struct {
	URL       string
	ExpiresAt time.Time
}

// initSubtitleCache starts the background expiration cleanup goroutine
func initSubtitleCache() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			subtitleMu.Lock()
			for imdbID, entry := range subtitleCache {
				if now.After(entry.ExpiresAt) {
					delete(subtitleCache, imdbID)
				}
			}
			subtitleMu.Unlock()
		}
	}()
}

// SyncSubtitles fetches subtitles from OpenSubtitles API and caches the result
func SyncSubtitles(imdbID string) {
	if imdbID == "" {
		return
	}

	// Check cache first (respect TTL) and prevent duplicate fetches
	subtitleMu.Lock()
	if entry, ok := subtitleCache[imdbID]; ok {
		if time.Now().Before(entry.ExpiresAt) {
			subtitleMu.Unlock()
			return
		}
		delete(subtitleCache, imdbID)
	}
	// Deduplicate: only one fetch per IMDB ID at a time
	if pendingFetch[imdbID] {
		subtitleMu.Unlock()
		return
	}
	pendingFetch[imdbID] = true
	subtitleMu.Unlock()

	osApiKey := os.Getenv("OPENSUBTITLES_API_KEY")
	if osApiKey == "" {
		subtitleMu.Lock()
		delete(pendingFetch, imdbID)
		subtitleMu.Unlock()
		return
	}

	urlStr := fmt.Sprintf("https://api.opensubtitles.com/api/v1/subtitles?imdb_id=%s&languages=en&sub_format=srt", imdbID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		subtitleMu.Lock()
		delete(pendingFetch, imdbID)
		subtitleMu.Unlock()
		return
	}
	req.Header.Set("Api-Key", osApiKey)
	req.Header.Set("Content-Type", "application/json")

	go func() {
		defer func() {
			subtitleMu.Lock()
			delete(pendingFetch, imdbID)
			subtitleMu.Unlock()
		}()

		resp, err := httpClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return
		}

		var result struct {
			Data []struct {
				ID           string `json:"id"`
				DownloadUrl  string `json:"download_url"`
				LanguageName string `json:"language_name"`
				Format       string `json:"format"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return
		}

		// Use first English SRT subtitle found
		for _, sub := range result.Data {
			if sub.DownloadUrl != "" && (sub.Format == "srt" || sub.Format == "") {
				subtitleMu.Lock()
				subtitleCache[imdbID] = subtitleCacheEntry{
					URL:       sub.DownloadUrl,
					ExpiresAt: time.Now().Add(subtitleTTL),
				}
				subtitleMu.Unlock()
				log.Printf("📝 Cached subtitle for %s from OpenSubtitles", imdbID)
				return
			}
		}
	}()
}
