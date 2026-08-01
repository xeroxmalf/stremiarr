package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LibraryItem represents an item in the Stremio user library.
type LibraryItem struct {
	ID      string `json:"_id"`  // IMDB ID e.g. tt1234567
	Type    string `json:"type"` // "movie" or "series"
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

func getStremioAuthPath() string     { return DataDir + "/stremio_auth.json" }
func getPrefetchHistoryPath() string { return DataDir + "/prefetch_history.json" }

func loadPrefetchHistory() {
	prefetchHistoryMu.Lock()
	defer prefetchHistoryMu.Unlock()
	data, err := os.ReadFile(getPrefetchHistoryPath())
	if err == nil {
		if err := json.Unmarshal(data, &prefetchHistory); err != nil {
			log.Printf("⚠️ Failed to parse prefetch history: %v", err)
		}
	}
}

func savePrefetchHistory() {
	prefetchHistoryMu.Lock()
	defer prefetchHistoryMu.Unlock()
	data, err := json.Marshal(prefetchHistory)
	if err == nil {
		if err := os.WriteFile(getPrefetchHistoryPath(), data, 0644); err != nil {
			log.Printf("⚠️ Failed to write prefetch history: %v", err)
		}
	}
}

// loadStremioAuth loads the saved auth key from DataDir.
func loadStremioAuth() string {
	data, err := os.ReadFile(getStremioAuthPath())
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

// saveStremioAuth saves the auth key to DataDir.
func saveStremioAuth(authKey string) {
	payload := map[string]string{"auth_key": authKey}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Printf("❌ [Prefetch] Failed to marshal stremio auth: %v", err)
		return
	}
	if err := os.WriteFile(getStremioAuthPath(), data, 0644); err != nil {
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
// Returns all unique infoHash values found across all sources, plus any HTTP stream URLs
// discovered during the scan (for validation).
func fetchHashesForItem(imdbID, mediaType string) (map[string]int, []string) {
	sourcesMu.Lock()
	enabled := make([]AddonSource, 0, len(addonSources))
	for _, src := range addonSources {
		if src.Enabled {
			enabled = append(enabled, src)
		}
	}
	sourcesMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	type result struct {
		hashes map[string]int
		urls   []string
	}

	ch := make(chan result, len(enabled)+1)

	for _, src := range enabled {
		go func(source AddonSource) {
			base := strings.TrimSuffix(source.URL, "/manifest.json")
			base = strings.TrimRight(base, "/")
			streamURL := fmt.Sprintf("%s/stream/%s/%s.json", base, mediaType, imdbID)

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

			hashes := make(map[string]int)
			var urls []string

			for _, stream := range parsed.Streams {
				title, _ := stream["title"].(string)
				name, _ := stream["name"].(string)
				fullText := title + "\n" + name

				seeders := -1 // Default to unknown
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

				// Capture HTTP stream URLs for validation
				if urlStr, ok := stream["url"].(string); ok && urlStr != "" && strings.HasPrefix(urlStr, "http") && !isSuspiciousURL(urlStr) {
					urls = append(urls, urlStr)
				}
			}
			log.Printf("[Prefetch] 📡 %s → %s: %d hashes, %d URLs", source.Name, streamURL, len(hashes), len(urls))
			ch <- result{hashes: hashes, urls: urls}
		}(src)
	}

	// Directly query Debrid Media Manager's global library using Proof-of-Work
	go func() {
		hashes := scrapeDMMDirectly(ctx, imdbID, mediaType)
		ch <- result{hashes: hashes}
	}()

	allHashes := make(map[string]int)
	seenURLs := make(map[string]bool)
	var allURLs []string
	for i := 0; i < len(enabled)+1; i++ {
		res := <-ch
		for h, seeders := range res.hashes {
			if existing, ok := allHashes[h]; !ok || seeders > existing {
				allHashes[h] = seeders
			}
			// Limit to 100 hashes total per item to avoid API abuse
			if len(allHashes) >= 100 {
				return allHashes, allURLs
			}
		}
		for _, u := range res.urls {
			if !seenURLs[u] {
				seenURLs[u] = true
				allURLs = append(allURLs, u)
			}
		}
	}
	return allHashes, allURLs
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
		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			log.Printf("⚠️ [Prefetch] Failed to create instantAvailability request: %v", err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
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

// scrapeDMMDirectly simulates a client querying Debrid Media Manager's search API using PoW
func scrapeDMMDirectly(ctx context.Context, imdbID, mediaType string) map[string]int {
	endpoint := "movie"
	if mediaType == "series" {
		endpoint = "show"
	}

	dmmProblemKey, solution := generateDMMToken()
	url := fmt.Sprintf(
		"https://debridmediamanager.com/api/torrents/%s?imdbId=%s&dmmProblemKey=%s&solution=%s",
		endpoint, imdbID, dmmProblemKey, solution,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Prefetch] ⚠️ DMM direct API returned %d", resp.StatusCode)
		return nil
	}

	var parsed struct {
		Results []struct {
			Hash string `json:"hash"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil
	}

	hashes := make(map[string]int)
	for _, res := range parsed.Results {
		h := strings.ToLower(strings.TrimSpace(res.Hash))
		if h != "" {
			hashes[h] = 100 // Default fake seeders for DMM
		}
	}

	log.Printf("[Prefetch] 📡 DMM Direct Search -> %s: %d hashes", url, len(hashes))
	return hashes
}

func generateHash(str string) string {
	hash1 := uint32(0xdeadbeef ^ len(str))
	hash2 := uint32(0x41c6ce57 ^ len(str))

	for i := 0; i < len(str); i++ {
		charCode := uint32(str[i])
		hash1 = (hash1 ^ charCode) * 2654435761
		hash2 = (hash2 ^ charCode) * 1597334677
		hash1 = (hash1 << 5) | (hash1 >> 27)
		hash2 = (hash2 << 5) | (hash2 >> 27)
	}

	hash1 += hash2 * 1566083941
	hash2 += hash1 * 2024237689

	return fmt.Sprintf("%x", hash1^hash2)
}

func combineHashes(hash1, hash2 string) string {
	halfLength := len(hash1) / 2
	firstPart1 := hash1[:halfLength]
	secondPart1 := hash1[halfLength:]
	firstPart2 := hash2[:halfLength]
	secondPart2 := hash2[halfLength:]

	var obfuscated string
	for i := 0; i < halfLength; i++ {
		obfuscated += string(firstPart1[i]) + string(firstPart2[i])
	}

	obfuscated += reverseString(secondPart2) + reverseString(secondPart1)
	return obfuscated
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func generateDMMToken() (string, string) {
	salt := "debridmediamanager.com%%fe7#td00rA3vHz%VmI"
	token := fmt.Sprintf("%x", rand.Uint32())
	timestamp := time.Now().Unix()

	tokenWithTimestamp := fmt.Sprintf("%s-%d", token, timestamp)
	tokenTimestampHash := generateHash(tokenWithTimestamp)
	tokenSaltHash := generateHash(salt + "-" + token)

	return tokenWithTimestamp, combineHashes(tokenTimestampHash, tokenSaltHash)
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
		hashes, streamURLs := fetchHashesForItem(item.ID, item.Type)

		// Persist and validate any discovered HTTP stream URLs
		if len(streamURLs) > 0 {
			validationQueued := 0
			for _, u := range streamURLs {
				if _, err := db.Exec("INSERT INTO stream_urls (url, is_valid, fail_count) VALUES (?, TRUE, 0) ON CONFLICT(url) DO NOTHING", u); err != nil {
					log.Printf("⚠️ [Prefetch] Failed to insert stream URL: %v", err)
				}
				select {
				case validateCh <- u:
					validationQueued++
				default:
				}
			}
			if validationQueued > 0 {
				log.Printf("🛡️ [Prefetch] Queued %d/%d stream URLs for validation from %s", validationQueued, len(streamURLs), itemLabel)
			}
		}

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
				// Only skip explicitly dead torrents (< 2 seeders)
				// If seeders == -1 (unknown), we give it a shot.
				if seeders != -1 && seeders < 2 {
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

		// Update Prometheus metrics
		metricPrefetchItems.Inc()
		for i := 0; i < cachedCount; i++ {
			metricPrefetchCached.Inc()
		}
		for i := 0; i < submittedCount; i++ {
			metricPrefetchSubmitted.Inc()
		}

		addPrefetchLog(fmt.Sprintf("✅ %s — cached:%d submitted:%d", itemLabel, cachedCount, submittedCount))

		time.Sleep(1 * time.Second)
	}

	addPrefetchLog(fmt.Sprintf("🏁 Done! Processed %d items", len(library)))
}
