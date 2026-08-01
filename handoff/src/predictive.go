package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var seriesRegex = regexp.MustCompile(`^(tt\d+):(\d+):(\d+)`)

var (
	predictiveMu  sync.Mutex
	predictiveSet = make(map[string]struct{}) // set of keys predicted within last hour
)

func PredictivePreCache(videoID string) {
	// videoID format: tt123456:1:1
	matches := seriesRegex.FindStringSubmatch(videoID)
	if len(matches) != 4 {
		return // Not an episode
	}

	imdbID := matches[1]
	season, _ := strconv.Atoi(matches[2])
	episode, _ := strconv.Atoi(matches[3])

	// Only predict next episode within a reasonable range
	if episode > 20 {
		return
	}
	nextEpisode := episode + 1
	nextKey := fmt.Sprintf("%s:%d:%d", imdbID, season, nextEpisode)

	// Rate limit: don't re-predict the same episode within 1 hour
	predictiveMu.Lock()
	if _, ok := predictiveSet[nextKey]; ok {
		predictiveMu.Unlock()
		return
	}
	predictiveSet[nextKey] = struct{}{}
	predictiveMu.Unlock()

	// Clean up prediction flag after 1 hour
	go func() {
		time.Sleep(1 * time.Hour)
		predictiveMu.Lock()
		delete(predictiveSet, nextKey)
		predictiveMu.Unlock()
	}()

	// Don't interfere with active prefetch job
	prefetchStatusMu.RLock()
	alreadyRunning := prefetchStatus.Running
	prefetchStatusMu.RUnlock()
	if alreadyRunning {
		return
	}

	log.Printf("🔮 Predictive Pre-Caching: User is looking at %s, triggering background scan for %s", videoID, nextKey)

	go func() {
		// Discover hashes for next episode using the IMDB ID
		hashes, streamURLs := fetchHashesForItem(imdbID, "series")

		// Queue any discovered stream URLs for validation (limit to avoid flooding)
		queued := 0
		for _, u := range streamURLs {
			if queued >= 10 {
				break
			}
			if _, err := db.Exec("INSERT INTO stream_urls (url, is_valid, fail_count) VALUES (?, TRUE, 0) ON CONFLICT(url) DO NOTHING", u); err != nil {
				log.Printf("⚠️ [Predictive] Failed to insert stream URL: %v", err)
			}
			select {
			case validateCh <- u:
				queued++
			default:
			}
		}

		if len(hashes) == 0 {
			log.Printf("🔮 Predictive Pre-Caching: No hashes found for %s", nextKey)
			return
		}

		// Filter for good seeds only (>= 5 seeders)
		var bestHashes []string
		for h, seeders := range hashes {
			if seeders >= 5 {
				bestHashes = append(bestHashes, h)
			}
		}

		if len(bestHashes) == 0 {
			return
		}

		// Check RD cache
		cached, _ := checkRDInstantAvailability(bestHashes)
		var targetHash string
		for _, h := range bestHashes {
			if cached[strings.ToLower(h)] {
				// Already cached!
				log.Printf("🔮 Predictive Pre-Caching: %s is already cached!", nextKey)
				return
			}
			if targetHash == "" {
				targetHash = h
			}
		}

		if targetHash != "" {
			log.Printf("🔮 Predictive Pre-Caching: Queueing %s (%s) to Debrid", nextKey, targetHash)
			err := rdAddMagnet("magnet:?xt=urn:btih:" + targetHash)
			if err != nil {
				if !strings.Contains(err.Error(), "451") {
					log.Printf("🔮 Predictive Pre-Caching: Failed to queue %s: %v", targetHash, err)
				}
			}
		}
	}()
}
