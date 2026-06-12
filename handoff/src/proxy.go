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

var seederRegex = regexp.MustCompile(`(?i)(?:👤|seeders:|s:)\s*(\d+)`)

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

func fetchFromSource(ctx context.Context, url string) (StreamFetcher, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return StreamFetcher{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return StreamFetcher{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	// Track bandwidth usage
	TrackBandwidth(len(body), url)

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

func fetchAndCacheStreams(targetURL string, config Config) ([]byte, error) {
	log.Printf("[Stream] 🔄 Fetching fresh upstream streams: %s", targetURL)

	// Start with the primary source
	sources := []string{targetURL}

	// Add enabled addon sources from configuration
	sourcesMu.Lock()
	enabledSources := make([]AddonSource, 0, len(addonSources))
	for _, src := range addonSources {
		if src.Enabled {
			enabledSources = append(enabledSources, src)
		}
	}
	sourcesMu.Unlock()

	// Add enabled sources (if not already in targetURL)
	for _, src := range enabledSources {
		alreadyIncluded := false
		for _, existing := range sources {
			if strings.Contains(existing, src.URL) || strings.Contains(src.URL, existing) {
				alreadyIncluded = true
				break
			}
		}
		if !alreadyIncluded && !strings.Contains(targetURL, src.URL) {
			sources = append(sources, src.URL)
		}
	}

	// Also add config-specified sources (if set)
	if config.TorrentioURL != "" && !strings.Contains(targetURL, config.TorrentioURL) {
		sources = append(sources, config.TorrentioURL)
	}
	if config.MediafusionURL != "" && !strings.Contains(targetURL, config.MediafusionURL) {
		sources = append(sources, config.MediafusionURL)
	}
	if config.StremthruURL != "" && !strings.Contains(targetURL, config.StremthruURL) {
		sources = append(sources, config.StremthruURL)
	}

	// Fetch from multiple sources in parallel
	type result struct {
		fetcher StreamFetcher
		err     error
	}

	ch := make(chan result, len(sources))
	var allFetcher StreamFetcher

	// 🚀 Fast-Fail: Drop upstream sources if they take more than 8 seconds.
	// This dramatically improves the UX for cache misses (the first ever load).
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	for _, src := range sources {
		go func(source string) {
			f, err := fetchFromSource(ctx, source)
			ch <- result{fetcher: f, err: err}
		}(src)
	}

	for range sources {
		res := <-ch
		if res.err == nil {
			allFetcher.Merge(res.fetcher)
		}
	}

	// Deduplicate streams
	seenURLs := make(map[string]bool)
	var deduped []interface{}
	
	// Phase 5: Compile blacklist regex once outside the loop for massive performance boost
	blacklistPattern := os.Getenv("STREAM_BLACKLIST_REGEX")
	var blacklistRegex *regexp.Regexp
	if blacklistPattern != "" {
		compiled, err := regexp.Compile("(?i)" + blacklistPattern)
		if err == nil {
			blacklistRegex = compiled
		} else {
			log.Printf("[Stream] ⚠️ Invalid STREAM_BLACKLIST_REGEX pattern: %v", err)
		}
	}

	for _, s := range allFetcher.Streams {
		if stream, ok := s.(map[string]interface{}); ok {
			if urlStr, ok := stream["url"].(string); ok {
				if isSuspiciousURL(urlStr) {
					log.Printf("[Stream] ✂️  Pre-filter redacted suspicious URL: %s", urlStr)
					continue
				}

				// Blacklist filtering (Phase 5)
				if blacklistRegex != nil {
					matched := false
					if title, ok := stream["title"].(string); ok {
						if blacklistRegex.MatchString(title) {
							matched = true
						}
					}
					if name, ok := stream["name"].(string); ok {
						if blacklistRegex.MatchString(name) {
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
				db.Exec("INSERT INTO stream_urls (url, is_valid, fail_count) VALUES (?, 1, 0) ON CONFLICT(url) DO NOTHING", urlStr)
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
	cache.SetStreamCache(targetURL, string(body), 7*24*time.Hour)

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
		w.Write(body)
		return
	}

	wrappedCount := 0
	redactedCount := 0
	seenURLs := make(map[string]struct{})
	const maxStreams = 30 // cap to reduce noise and improve UX

	if streams, ok := result["streams"].([]interface{}); ok {
		var finalStreams []interface{}

		host := "http://" + r.Host
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			host = "https://" + r.Host
		}

		for _, s := range streams {
			if wrappedCount >= maxStreams {
				redactedCount++
				break
			}

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

			// Consider a stream "stale" if last validated > 30 minutes ago
			stale := false
			if err == nil && !lastValidated.IsZero() && time.Since(lastValidated) > 30*time.Minute {
				stale = true
			}

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
				// If stale AND already had some strikes, treat as suspect
				if stale && failCount > 0 {
					log.Printf("[Stream] ✂️  Redacting stream (stale + prior strikes): %s", urlStr)
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
							go rdAddMagnet(hash)
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
	w.Write(modifiedBody)
	log.Printf("[Stream] ✅ Served %d streams (%d actively redacted, incl. junk/dead/dupes)", wrappedCount, redactedCount)
}

// --------------------------------
// PLAY FAILOVER LOGIC
// --------------------------------

func proxyRequest(w http.ResponseWriter, r *http.Request, addonURL string, subPath string, rawQuery string) {
	if strings.Contains(subPath, "series/tt") || strings.Contains(subPath, "movie/tt") {
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(subPath, "series/"), "movie/"), ".")
		if len(parts) > 0 {
			if strings.Contains(subPath, "series/tt") {
				PredictivePreCache(parts[0])
			}
			SyncSubtitles(strings.Split(parts[0], ":")[0]) // pass imdbID
		}
	}

	targetURL := getTargetURL(addonURL, subPath, r.URL.RawQuery)

	if body, headers, statusCode, err := cache.GetCatalogCache(targetURL); err == nil {
		if strings.Contains(subPath, "meta/") {
			body = AugmentMetadata(body)
		}
		for k, v := range headers {
			w.Header()[k] = v
		}
		w.WriteHeader(statusCode)
		w.Write(body)
		log.Printf("[Catalog] ⚡ Cache hit for: %s", targetURL)
		return
	}

	log.Printf("[Catalog] 🔀 Proxying meta/catalog request to: %s", targetURL)

	resp, err := httpClient.Get(targetURL)
	if err != nil {
		log.Printf("[Catalog] ❌ Proxy request failed: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	cache.SetCatalogCache(targetURL, body, resp.Header.Clone(), resp.StatusCode, 5*time.Minute)

	if strings.Contains(subPath, "meta/") {
		body = AugmentMetadata(body)
	}

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
