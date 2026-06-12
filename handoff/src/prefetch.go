package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LibraryItem represents an item in the Stremio user library.
type LibraryItem struct {
	ID      string `json:"_id"`   // IMDB ID e.g. tt1234567
	Type    string `json:"type"`  // "movie" or "series"
	Name    string `json:"name"`
	Removed bool   `json:"removed"`
}

// PrefetchStatus tracks the state of a running (or last completed) prefetch job.
type PrefetchStatus struct {
	Running       bool      `json:"running"`
	Total         int       `json:"total"`
	Processed     int       `json:"processed"`
	AlreadyCached int       `json:"already_cached"`
	Submitted     int       `json:"submitted"`
	Failed        int       `json:"failed"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	CurrentItem   string    `json:"current_item"`
	Log           []string  `json:"log"`
}

var (
	prefetchStatus   = &PrefetchStatus{}
	prefetchStatusMu sync.RWMutex
	prefetchCancel   context.CancelFunc
	prefetchCancelMu sync.Mutex

	// In-memory Stremio auth (persisted to disk for restarts)
	stremioAuthKey string
	stremioAuthMu  sync.RWMutex

	prefetchHistory   = make(map[string]bool)
	prefetchHistoryMu sync.Mutex
)

const stremioAuthPath = "/data/stremio_auth.json"
const prefetchHistoryPath = "/data/prefetch_history.json"

func loadPrefetchHistory() {
	prefetchHistoryMu.Lock()
	defer prefetchHistoryMu.Unlock()
	data, err := os.ReadFile(prefetchHistoryPath)
	if err == nil {
		json.Unmarshal(data, &prefetchHistory)
	}
}

func savePrefetchHistory() {
	prefetchHistoryMu.Lock()
	defer prefetchHistoryMu.Unlock()
	os.MkdirAll("/data", 0755)
	data, err := json.Marshal(prefetchHistory)
	if err == nil {
		os.WriteFile(prefetchHistoryPath, data, 0644)
	}
}

// loadStremioAuth loads the saved auth key from /data/stremio_auth.json.
func loadStremioAuth() string {
	data, err := os.ReadFile(stremioAuthPath)
	if err != nil {
		return ""
	}
	var payload struct {
		AuthKey string `json:"auth_key"`
	}
	if json.Unmarshal(data, &payload) == nil {
		return payload.AuthKey
	}
	return ""
}

// saveStremioAuth saves the auth key to /data/stremio_auth.json.
func saveStremioAuth(authKey string) {
	os.MkdirAll("/data", 0755)
	payload := map[string]string{"auth_key": authKey}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Printf("❌ [Prefetch] Failed to marshal stremio auth: %v", err)
		return
	}
	if err := os.WriteFile(stremioAuthPath, data, 0644); err != nil {
		log.Printf("❌ [Prefetch] Failed to write stremio_auth.json: %v", err)
	}
}

// stremioLogin authenticates with Stremio and returns an auth key.
// POST https://api.strem.io/api/login
func stremioLogin(email, password string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
		"type":     "Login",
	})
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Post("https://api.strem.io/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("stremio login request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Result struct {
			AuthKey string `json:"authKey"`
		} `json:"result"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse stremio login response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("stremio login error: %s", result.Error)
	}
	if result.Result.AuthKey == "" {
		return "", fmt.Errorf("stremio login returned empty auth key (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return result.Result.AuthKey, nil
}

// fetchStremioLibrary fetches the user's library from Stremio API.
// POST https://api.strem.io/api/datastoreGet
func fetchStremioLibrary(authKey string) ([]LibraryItem, error) {
	body, err := json.Marshal(map[string]interface{}{
		"authKey":    authKey,
		"collection": "libraryItem",
		"ids":        []string{},
		"all":        true,
	})
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Post("https://api.strem.io/api/datastoreGet", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("stremio datastoreGet request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Result []LibraryItem `json:"result"`
		Error  string        `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse stremio library response: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("stremio library error: %s", result.Error)
	}

	// Filter out removed items and non-movie/series types
	var filtered []LibraryItem
	for _, item := range result.Result {
		if item.Removed {
			continue
		}
		if item.Type != "movie" && item.Type != "series" {
			continue
		}
		if item.ID == "" {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

// fetchHashesForItem queries all enabled addon sources for a given IMDB ID.
// Constructs stream URL by replacing /manifest.json with /stream/{type}/{imdbId}.json
// Returns all unique infoHash values found across all sources.
// NOTE: does NOT use StreamFetcher.Add() because that filters to HTTP-only streams.
func fetchHashesForItem(imdbID, mediaType string) map[string]int {
	sourcesMu.Lock()
	enabled := make([]AddonSource, 0, len(addonSources))
	for _, src := range addonSources {
		if src.Enabled {
			enabled = append(enabled, src)
		}
	}
	sourcesMu.Unlock()

	if len(enabled) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	type result struct {
		hashes map[string]int
	}

	ch := make(chan result, len(enabled))

	for _, src := range enabled {
		go func(source AddonSource) {
			base := strings.TrimSuffix(source.URL, "/manifest.json")
			base = strings.TrimRight(base, "/")
			streamURL := fmt.Sprintf("%s/stream/%s/%s.json", base, mediaType, imdbID)

			// Fetch raw JSON directly — do NOT use fetchFromSource/StreamFetcher
			// because StreamFetcher.Add() filters to HTTP-url streams only, dropping infoHashes.
			req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
			if err != nil {
				ch <- result{}
				return
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				ch <- result{}
				return
			}
			defer resp.Body.Close()

			var parsed struct {
				Streams []map[string]interface{} `json:"streams"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
				ch <- result{}
				return
			}

			var hashes map[string]int = make(map[string]int)

			for _, stream := range parsed.Streams {
				title, _ := stream["title"].(string)
				name, _ := stream["name"].(string)
				fullText := title + "\n" + name
				
				seeders := 0
				m := seederRegex.FindStringSubmatch(fullText)
				if len(m) > 1 {
					seeders, _ = strconv.Atoi(m[1])
				}

				hash := ""
				// Primary: standard infoHash field
				if ih, ok := stream["infoHash"].(string); ok && ih != "" {
					hash = strings.ToLower(strings.TrimSpace(ih))
				} else if bh, ok := stream["behaviorHints"].(map[string]interface{}); ok {
					// Fallback: some addons encode hash in behaviorHints.bingeGroup (first 40 chars)
					if bg, ok := bh["bingeGroup"].(string); ok && len(bg) >= 40 {
						hash = strings.ToLower(bg[:40])
					}
				}

				if hash != "" {
					if existing, ok := hashes[hash]; !ok || seeders > existing {
						hashes[hash] = seeders
					}
				}
			}
			log.Printf("[Prefetch] 📡 %s → %s: %d hashes", source.Name, streamURL, len(hashes))
			ch <- result{hashes: hashes}
		}(src)
	}

	allHashes := make(map[string]int)
	for i := 0; i < len(enabled); i++ {
		res := <-ch
		for h, seeders := range res.hashes {
			if existing, ok := allHashes[h]; !ok || seeders > existing {
				allHashes[h] = seeders
			}
			// Limit to 100 hashes total per item to avoid API abuse
			if len(allHashes) >= 100 {
				return allHashes
			}
		}
	}
	return allHashes
}

// checkRDInstantAvailability checks which hashes are instantly available (cached) on RD.
// GET https://api.real-debrid.com/rest/1.0/torrents/instantAvailability/{hash1}/{hash2}/...
// Batches into groups of 40 hashes per request.
// Returns map[lowercase_hash]bool
func checkRDInstantAvailability(hashes []string) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(hashes) == 0 {
		return result, nil
	}

	token := getRdApiKey()
	if token == "" {
		return result, nil
	}

	const batchSize = 40
	for i := 0; i < len(hashes); i += batchSize {
		end := i + batchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		chunk := hashes[i:end]

		urlStr := "https://api.real-debrid.com/rest/1.0/torrents/instantAvailability/" + strings.Join(chunk, "/")
		req, _ := http.NewRequest("GET", urlStr, nil)
		resp, err := rdDo(req)
		if err != nil {
			log.Printf("⚠️ RD Availability check failed for chunk: %v", err)
			continue // try next chunk instead of failing entirely
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Response format:
		// { "HASH": {"rd": [{...}]}, "HASH2": {} }
		// A hash is cached if its value has a non-empty "rd" array.
		var parsed map[string]interface{}
		if err := json.Unmarshal(body, &parsed); err != nil {
			log.Printf("❌ [Prefetch] Failed to parse instantAvailability response: %v. Raw body: %s", err, string(body))
			continue
		}

		for hash, val := range parsed {
			lh := strings.ToLower(hash)
			// RD might return "Invalid hash" string for bad hashes, so we type assert the map
			valMap, isMap := val.(map[string]interface{})
			if !isMap {
				result[lh] = false
				continue
			}
			
			if rdArr, ok := valMap["rd"].([]interface{}); ok && len(rdArr) > 0 {
				result[lh] = true
			} else {
				result[lh] = false
			}
		}
	}
	return result, nil
}

// addPrefetchLog appends a message to the status log (max 200 entries).
func addPrefetchLog(msg string) {
	prefetchStatusMu.Lock()
	defer prefetchStatusMu.Unlock()
	ts := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", ts, msg)
	prefetchStatus.Log = append(prefetchStatus.Log, entry)
	if len(prefetchStatus.Log) > 200 {
		prefetchStatus.Log = prefetchStatus.Log[len(prefetchStatus.Log)-200:]
	}
}

// runPrefetchJob is the main goroutine that processes the library.
func runPrefetchJob(ctx context.Context, authKey string, limit int, force bool) {
	defer func() {
		prefetchStatusMu.Lock()
		prefetchStatus.Running = false
		prefetchStatusMu.Unlock()
		log.Printf("🎬 [Prefetch] Job finished")
	}()

	addPrefetchLog("🎬 Fetching Stremio library...")
	log.Printf("🎬 [Prefetch] Starting prefetch job")

	if force {
		addPrefetchLog("⚠️ Force mode enabled: wiping prefetch history")
		log.Printf("⚠️ [Prefetch] Wiping prefetch history for full rescan")
		prefetchHistoryMu.Lock()
		prefetchHistory = make(map[string]bool)
		prefetchHistoryMu.Unlock()
		savePrefetchHistory()
	} else {
		loadPrefetchHistory()
	}

	library, err := fetchStremioLibrary(authKey)
	if err != nil {
		addPrefetchLog(fmt.Sprintf("❌ Failed to fetch library: %v", err))
		log.Printf("❌ [Prefetch] Failed to fetch library: %v", err)
		return
	}

	if limit > 0 && limit < len(library) {
		library = library[:limit]
	}

	prefetchStatusMu.Lock()
	prefetchStatus.Total = len(library)
	prefetchStatus.Processed = 0
	prefetchStatus.AlreadyCached = 0
	prefetchStatus.Submitted = 0
	prefetchStatus.Failed = 0
	prefetchStatusMu.Unlock()

	addPrefetchLog(fmt.Sprintf("📚 Found %d library items to process", len(library)))
	log.Printf("📚 [Prefetch] Processing %d library items", len(library))

	for _, item := range library {
		// Check for cancellation
		select {
		case <-ctx.Done():
			addPrefetchLog("⏹ Job cancelled by user")
			log.Printf("⏹ [Prefetch] Cancelled")
			return
		default:
		}

		prefetchHistoryMu.Lock()
		if prefetchHistory[item.ID] {
			prefetchHistoryMu.Unlock()
			prefetchStatusMu.Lock()
			prefetchStatus.Processed++
			prefetchStatusMu.Unlock()
			continue
		}
		prefetchHistoryMu.Unlock()

		itemLabel := fmt.Sprintf("%s (%s)", item.Name, item.ID)

		prefetchStatusMu.Lock()
		prefetchStatus.CurrentItem = itemLabel
		prefetchStatusMu.Unlock()

		addPrefetchLog(fmt.Sprintf("🔍 Checking: %s", itemLabel))
		log.Printf("🔍 [Prefetch] Processing: %s [%s]", item.Name, item.ID)

		// 1. Fetch hashes from all enabled addon sources
		hashes := fetchHashesForItem(item.ID, item.Type)

		if len(hashes) == 0 {
			addPrefetchLog(fmt.Sprintf("⚠️  No hashes found for %s", itemLabel))
			log.Printf("⚠️  [Prefetch] No hashes found for %s", item.ID)

			prefetchHistoryMu.Lock()
			prefetchHistory[item.ID] = true
			prefetchHistoryMu.Unlock()
			savePrefetchHistory()

			prefetchStatusMu.Lock()
			prefetchStatus.Processed++
			prefetchStatusMu.Unlock()
			continue
		}

		var hashList []string
		for h := range hashes {
			hashList = append(hashList, h)
		}

		// 2. Check RD instant availability
		availability, _ := checkRDInstantAvailability(hashList)

		cachedCount := 0
		submittedCount := 0

		for hash, seeders := range hashes {
			if isCached, ok := availability[hash]; ok && isCached {
				// Already on RD — bust the stream_cache so next Stremio request gets fresh RD links
				_, dbErr := db.Exec(
					"DELETE FROM stream_cache WHERE request_id LIKE ?",
					"%/"+item.Type+"/"+item.ID+"%",
				)
				if dbErr != nil {
					log.Printf("⚠️ [Prefetch] stream_cache bust error for %s: %v", item.ID, dbErr)
				}
				cachedCount++
			} else {
				// Only submit to RD if there are >= 5 seeders
				if seeders < 5 {
					continue
				}

				// Limit to 3 uncached hashes submitted per item
				if submittedCount >= 3 {
					continue
				}

				// Not cached — submit to RD
				time.Sleep(1 * time.Second)
				err := rdAddMagnet("magnet:?xt=urn:btih:" + hash)
				if err == nil {
					addPrefetchLog(fmt.Sprintf("🚀 Successfully queued %s to Debrid", hash))
					SyncArrStack("/links/" + hash) // Notify Radarr/Sonarr
				} else {
					if strings.Contains(err.Error(), "451") {
						continue // Infringing file, silently skip
					}
					if strings.Contains(err.Error(), "429") {
						addPrefetchLog("⏳ RD Rate Limited (429). Pausing for 5 minutes...")
						log.Printf("⏳ [Prefetch] RD Rate Limited (429). Pausing for 5 minutes...")
						
						// Backoff for 5 minutes, but wake up if cancelled
						select {
						case <-ctx.Done():
							addPrefetchLog("⏹ Job cancelled by user during rate limit backoff")
							log.Printf("⏹ [Prefetch] Cancelled during backoff")
							return
						case <-time.After(5 * time.Minute):
						}
						continue
					}
					// Some other error, just skip
					continue
				}
				
				submittedCount++
			}
		}

		addPrefetchLog(fmt.Sprintf("✅ %s cached: %d, submitted: %d", itemLabel, cachedCount, submittedCount))
		log.Printf("✅ [Prefetch] %s finished. Cached: %d, Submitted: %d", item.ID, cachedCount, submittedCount)

		prefetchHistoryMu.Lock()
		prefetchHistory[item.ID] = true
		prefetchHistoryMu.Unlock()
		savePrefetchHistory()

		prefetchStatusMu.Lock()
		prefetchStatus.Processed++
		prefetchStatus.AlreadyCached += cachedCount
		prefetchStatus.Submitted += submittedCount
		prefetchStatusMu.Unlock()

		addPrefetchLog(fmt.Sprintf("✅ %s — cached:%d submitted:%d", itemLabel, cachedCount, submittedCount))

		time.Sleep(1 * time.Second)
	}

	addPrefetchLog(fmt.Sprintf("🏁 Done! Processed %d items", len(library)))
}
