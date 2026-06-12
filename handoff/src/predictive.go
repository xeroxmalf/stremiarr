package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
)

var seriesRegex = regexp.MustCompile(`^(tt\d+):(\d+):(\d+)`)

func PredictivePreCache(videoID string) {
	// videoID format: tt123456:1:1
	matches := seriesRegex.FindStringSubmatch(videoID)
	if len(matches) != 4 {
		return // Not an episode
	}

	imdbID := matches[1]
	season, _ := strconv.Atoi(matches[2])
	episode, _ := strconv.Atoi(matches[3])

	nextEpisode := episode + 1
	nextVideoID := fmt.Sprintf("%s:%d:%d", imdbID, season, nextEpisode)
	
	log.Printf("🔮 Predictive Pre-Caching: User is looking at %s, triggering background scan for %s", videoID, nextVideoID)
	
	go func() {
		// Discover hashes for next episode
		hashes := fetchHashesForItem(nextVideoID, "series")
		if len(hashes) == 0 {
			log.Printf("🔮 Predictive Pre-Caching: No hashes found for %s", nextVideoID)
			return
		}

		// Filter for good seeds
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
				log.Printf("🔮 Predictive Pre-Caching: %s is already cached!", nextVideoID)
				return
			}
			if targetHash == "" {
				targetHash = h
			}
		}

		if targetHash != "" {
			log.Printf("🔮 Predictive Pre-Caching: Queueing %s (%s) to Debrid", nextVideoID, targetHash)
			err := rdAddMagnet("magnet:?xt=urn:btih:" + targetHash)
			if err != nil {
				log.Printf("🔮 Predictive Pre-Caching: Failed to queue %s: %v", targetHash, err)
			}
		}
	}()
}
