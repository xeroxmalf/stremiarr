package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	seederRegex          = regexp.MustCompile(`(?i)(?:👤|seeders:|s:|s\s+|seeders\s+)\s*(\d+)`)
	globalBlacklistRegex *regexp.Regexp
)

func init() {
	if pattern := os.Getenv("STREAM_BLACKLIST_REGEX"); pattern != "" {
		compiled, err := regexp.Compile("(?i)" + pattern)
		if err == nil {
			globalBlacklistRegex = compiled
		} else {
			log.Printf("[Stream] ⚠️ Invalid STREAM_BLACKLIST_REGEX pattern: %v", err)
		}
	}
}

func getTargetURL(addonURL, subPath, rawQuery string) string {
	base := strings.TrimSuffix(addonURL, "/manifest.json")
	base = strings.TrimRight(base, "/")
	target := base + "/" + subPath
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	return target
}
func (f *StreamFetcher) Add(stream map[string]interface{}) {
	if urlStr, ok := stream["url"].(string); ok && urlStr != "" && strings.HasPrefix(urlStr, "http") && !isSuspiciousURL(urlStr) {
		f.Streams = append(f.Streams, stream)
	}
}

func (f *StreamFetcher) Merge(other StreamFetcher) {
	f.Streams = append(f.Streams, other.Streams...)
}

func fetchFromSource(ctx context.Context, sourceURL string) (StreamFetcher, error) {
	select {
	case <-ctx.Done():
		return StreamFetcher{}, ctx.Err()
	default:
	}

	req, err := http.NewRequestWithContext(ctx, "GET", sourceURL, nil)
	if err != nil {
		return StreamFetcher{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return StreamFetcher{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return StreamFetcher{}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return StreamFetcher{}, err
	}

	// Track bandwidth
	TrackBandwidth(len(body), sourceURL)

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return StreamFetcher{}, nil
	}

	fetcher := StreamFetcher{}
	if streams, ok := result["streams"].([]interface{}); ok {
		for _, s := range streams {
			if stream, ok := s.(map[string]interface{}); ok {
				fetcher.Add(stream)
			}
		}
	}
	return fetcher, nil
}

func fetchAndCacheStreams(ctx context.Context, targetURL string, subPath string, rawQuery string, config Config) ([]byte, error) {
	log.Printf("[Stream] 🔄 Fetching fresh upstream streams: %s", targetURL)

	// Build source list
	sources := []string{targetURL}

	sourcesMu.Lock()
	enabledSources := make([]AddonSource, 0, len(addonSources))
	for _, src := range addonSources {
		if src.Enabled {
			enabledSources = append(enabledSources, src)
		}
	}
	sourcesMu.Unlock()

	// Dedup helper
	addSource := func(u string) {
		for _, existing := range sources {
			if existing == u {
				return
			}
		}
		sources = append(sources, u)
	}

	for _, src := range enabledSources {
		addSource(getTargetURL(src.URL, subPath, rawQuery))
	}

	if config.TorrentioURL != "" {
		addSource(getTargetURL(config.TorrentioURL, subPath, rawQuery))
	}
	if config.MediafusionURL != "" {
		addSource(getTargetURL(config.MediafusionURL, subPath, rawQuery))
	}
	if config.StremthruURL != "" {
		addSource(getTargetURL(config.StremthruURL, subPath, rawQuery))
	}

	// Parallel fetch with context
	type result struct {
		fetcher StreamFetcher
		err     error
		source  string
	}

	// Semaphore to limit concurrent fetches (prevents overwhelming the client)
	maxConcurrent := 4
	if len(sources) < maxConcurrent {
		maxConcurrent = len(sources)
	}
	sem := make(chan struct{}, maxConcurrent)

	ch := make(chan result, len(sources))
	var allFetcher StreamFetcher
	var fetchErrors []string

	for _, src := range sources {
		go func(source string) {
			sem <- struct{}{}
			defer func() { <-sem }()

			f, err := fetchFromSource(ctx, source)
			ch <- result{fetcher: f, err: err, source: source}
		}(src)
	}

	for range sources {
		res := <-ch
		if res.err != nil {
			// Don't log context cancellations as errors
			if res.err != context.DeadlineExceeded && res.err != context.Canceled {
				fetchErrors = append(fetchErrors, res.source+": "+res.err.Error())
			}
			continue
		}
		if len(res.fetcher.Streams) > 0 {
			allFetcher.Merge(res.fetcher)
		}
	}

	if len(fetchErrors) > 0 && len(allFetcher.Streams) == 0 {
		log.Printf("[Stream] ⚠️ All sources failed: %v", fetchErrors)
	}

	// Deduplicate streams
	seenURLs := make(map[string]bool)
	var deduped []interface{}

	// Phase 5: Blacklist filtering (using global compiled regex)

	for _, s := range allFetcher.Streams {
		if stream, ok := s.(map[string]interface{}); ok {
			if urlStr, ok := stream["url"].(string); ok {
				if isSuspiciousURL(urlStr) {
					log.Printf("[Stream] ✂️  Pre-filter redacted suspicious URL: %s", urlStr)
					continue
				}

				// Blacklist filtering (Phase 5)
				if globalBlacklistRegex != nil {
					matched := false
					if title, ok := stream["title"].(string); ok {
						if globalBlacklistRegex.MatchString(title) {
							matched = true
						}
					}
					if name, ok := stream["name"].(string); ok {
						if globalBlacklistRegex.MatchString(name) {
							matched = true
						}
					}
					if matched {
						log.Printf("[Stream] 🚫 Filtered stream matching blacklist")
						continue
					}
				}

				if !seenURLs[urlStr] {
					seenURLs[urlStr] = true
					deduped = append(deduped, stream)
				}
			}
		} else {
			deduped = append(deduped, s)
		}
	}

	// Persist to DB
	for _, s := range deduped {
		if stream, ok := s.(map[string]interface{}); ok {
			if urlStr, ok := stream["url"].(string); ok && urlStr != "" {
				if _, err := db.Exec("INSERT INTO stream_urls (url, is_valid, fail_count) VALUES (?, TRUE, 0) ON CONFLICT(url) DO NOTHING", urlStr); err != nil {
					log.Printf("⚠️ Failed to insert stream URL: %v", err)
				}
				select {
				case validateCh <- urlStr:
				default:
				}
			}
		}
	}

	// Build final JSON
	finalResult := map[string]interface{}{
		"streams": deduped,
	}

	body, _ := json.Marshal(finalResult)
	if err := cache.SetStreamCache(targetURL, string(body), 7*24*time.Hour); err != nil {
		log.Printf("⚠️ Failed to set stream cache: %v", err)
	}

	log.Printf("[Stream] ✅ Fetched %d streams from %d sources", len(deduped), len(sources))
	FireWebhook("streams_fetched", fmt.Sprintf("Fetched %d streams for target %s", len(deduped), targetURL))
	return body, nil
}

func isSuspiciousURL(u string) bool {
	// Filter obviously junk or test links
	low := strings.ToLower(u)
	for _, p := range []string{
		"example.com",
		"example.org",
		"test.stream",
		"stream-test",
		"placeholder",
		"click-here",
		"watch-here",
		"demo.link",
		"invalid-stream",
	} {
		if strings.Contains(low, p) {
			return true
		}
	}
	// Filter out non-HTTP(S) links
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return true
	}
	return false
}

func serveStreamsJSON(w http.ResponseWriter, r *http.Request, body []byte, idOrConfig string) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			log.Printf("⚠️ Failed to write response: %v", err)
		}
		return
	}

	wrappedCount := 0
	redactedCount := 0
	seenURLs := make(map[string]struct{})
	if streams, ok := result["streams"].([]interface{}); ok {
		var finalStreams []interface{}

		host := "http://" + r.Host
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			host = "https://" + r.Host
		}

		for _, s := range streams {
			stream, ok := s.(map[string]interface{})
			if !ok {
				finalStreams = append(finalStreams, s)
				continue
			}

			urlStr, hasUrl := stream["url"].(string)
			if !hasUrl || urlStr == "" || !strings.HasPrefix(urlStr, "http") {
				// Non-HTTP streams (infoHash / magnet P2P): pass through and queue for RD caching
				stream["_heuristic_score"] = 0
				stream["_is_infohash"] = true
				finalStreams = append(finalStreams, stream)
				continue
			}

			// Basic suspicious URL filter
			if isSuspiciousURL(urlStr) {
				log.Printf("[Stream] ✂️  Redacting suspicious URL: %s", urlStr)
				redactedCount++
				continue
			}

			// Deduplication: keep only one stream per unique URL
			if _, exists := seenURLs[urlStr]; exists {
				log.Printf("[Stream] ✂️  Dedup: skipping duplicate stream URL: %s", urlStr)
				redactedCount++
				continue
			}
			seenURLs[urlStr] = struct{}{}

			// Strike/Validation Gate Check
			var failCount int
			var successCount int
			var isValid bool
			var lastValidated time.Time
			err := db.QueryRow("SELECT fail_count, success_count, is_valid, last_validated FROM stream_urls WHERE url = ?", urlStr).Scan(&failCount, &successCount, &isValid, &lastValidated)

			if err == nil {
				// Hard block if too many strikes
				if failCount >= 3 {
					log.Printf("[Stream] ✂️  Redacting stream (3+ strikes): %s", urlStr)
					redactedCount++
					continue
				}
				// Hard block if explicitly marked invalid
				if !isValid {
					log.Printf("[Stream] ✂️  Redacting stream (Invalid/Dead): %s", urlStr)
					redactedCount++
					continue
				}
			}

			// Wrap URL via Handoff play endpoint
			stream["url"] = fmt.Sprintf("%s/%s/play?link=%s", host, idOrConfig, url.QueryEscape(urlStr))

			if name, ok := stream["name"]; ok {
				stream["name"] = fmt.Sprintf("🚀 Handoff\n%v", name)
			} else {
				stream["name"] = "🚀 Handoff"
			}

			stream["_heuristic_score"] = successCount - failCount

			wrappedCount++
			finalStreams = append(finalStreams, stream)
		}

		sort.SliceStable(finalStreams, func(i, j int) bool {
			getScore := func(item interface{}) int {
				m, ok := item.(map[string]interface{})
				if !ok {
					return 0
				}
				score, ok := m["_heuristic_score"].(int)
				if !ok {
					return 0
				}
				return score
			}
			return getScore(finalStreams[i]) > getScore(finalStreams[j])
		})
		result["streams"] = finalStreams

		// If there were zero RD-wrapped HTTP streams but there are infoHash torrents,
		// queue them on Real-Debrid so they'll be cached for next time.
		if wrappedCount == 0 {
			for _, s := range finalStreams {
				if m, ok := s.(map[string]interface{}); ok {
					if m["_is_infohash"] == true {
						title, _ := m["title"].(string)
						name, _ := m["name"].(string)
						fullText := title + "\n" + name

						seeders := 0
						matches := seederRegex.FindStringSubmatch(fullText)
						if len(matches) > 1 {
							seeders, _ = strconv.Atoi(matches[1])
						}

						if seeders < 5 {
							continue
						}

						hash := ""
						if ih, ok := m["infoHash"].(string); ok && ih != "" {
							hash = ih
						} else if bh, ok := m["behaviorHints"].(map[string]interface{}); ok {
							if bg, ok := bh["bingeGroup"].(string); ok && len(bg) >= 40 {
								hash = bg[:40]
							}
						}
						if hash != "" {
							log.Printf("[RD] 📤 Queuing uncached torrent for RD download: %s (Seeders: %d)", hash, seeders)
							go func(h string) {
								if err := rdAddMagnet(h); err != nil {
									log.Printf("⚠️ Failed to add magnet: %v", err)
								}
							}(hash)
						}
					}
				}
			}
		}

		// Clean up internal tracking fields before sending to client
		for _, s := range finalStreams {
			if m, ok := s.(map[string]interface{}); ok {
				delete(m, "_heuristic_score")
				delete(m, "_is_infohash")
			}
		}
	}

	modifiedBody, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(modifiedBody); err != nil {
		log.Printf("⚠️ Failed to write response: %v", err)
	}
	log.Printf("[Stream] ✅ Served %d streams (%d actively redacted, incl. junk/dead/dupes)", wrappedCount, redactedCount)
}

// --------------------------------
// PLAY FAILOVER LOGIC
// --------------------------------

func proxyRequest(w http.ResponseWriter, r *http.Request, addonURL string, subPath string, rawQuery string) {
	// Trigger predictive prefetch and subtitle sync for series/movie metadata
	if strings.Contains(subPath, "series/tt") || strings.Contains(subPath, "movie/tt") {
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(subPath, "series/"), "movie/"), ".")
		if len(parts) > 0 {
			if strings.Contains(subPath, "series/tt") {
				PredictivePreCache(parts[0])
			}
			// Extract IMDB ID for subtitle sync
			if imdbID := strings.Split(parts[0], ":")[0]; strings.HasPrefix(imdbID, "tt") {
				SyncSubtitles(imdbID)
			}
		}
	}

	targetURL := getTargetURL(addonURL, subPath, r.URL.RawQuery)

	// Check catalog cache first
	if body, headers, statusCode, err := cache.GetCatalogCache(targetURL); err == nil {
		if strings.Contains(subPath, "meta/") {
			body = AugmentMetadata(body)
		}
		for k, v := range headers {
			w.Header()[k] = v
		}
		w.WriteHeader(statusCode)
		if _, err := w.Write(body); err != nil {
			log.Printf("⚠️ Failed to write cached response: %v", err)
		}
		return
	}

	// Proxy the request with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, r.Body)
	if err != nil {
		log.Printf("[Catalog] ❌ Failed to create request: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// Copy relevant headers
	for k, vv := range r.Header {
		if k != "Host" && k != "Connection" && k != "Content-Length" {
			// Copy slice values to avoid aliasing the original header
			req.Header[k] = append([]string(nil), vv...)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[Catalog] ❌ Proxy request failed: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Catalog] ❌ Failed to read response: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// Cache successful responses
	if resp.StatusCode == 200 {
		if err := cache.SetCatalogCache(targetURL, body, resp.Header.Clone(), resp.StatusCode, 5*time.Minute); err != nil {
			log.Printf("⚠️ Failed to cache catalog response: %v", err)
		}
	}

	if strings.Contains(subPath, "meta/") {
		body = AugmentMetadata(body)
	}

	// Copy response headers (clone values to avoid shared slices)
	for k, vv := range resp.Header {
		w.Header()[k] = append([]string(nil), vv...)
	}
	w.WriteHeader(resp.StatusCode)
	// Skip body write for HEAD requests and no-body status codes (204, 304)
	if r.Method == "HEAD" || resp.StatusCode == 204 || resp.StatusCode == 304 {
		return
	}
	if _, err := w.Write(body); err != nil {
		log.Printf("⚠️ Failed to write proxied response: %v", err)
	}
}
