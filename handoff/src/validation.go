package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// --- VALIDATION WORKER POOL ---
var validateCh = make(chan string, 1000)

func initValidationPool() {
	// Reduced from 3 to 2 to prevent triggering RD's rate limits
	for i := 0; i < 2; i++ {
		go validationWorker()
	}
	go revalidationSweep()
	log.Printf("🛡️ API Validation Pool initialized (2 concurrent workers with rate limits + background sweep)")
}

// revalidationSweep periodically queries for stale streams and feeds them
// into validateCh so they get re-checked even without user traffic.
func revalidationSweep() {
	// Initial delay to let the system settle after startup
	time.Sleep(2 * time.Minute)

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	runSweep := func() {
		rows, err := db.Query(
			"SELECT url FROM stream_urls WHERE last_validated IS NULL OR last_validated < ? ORDER BY last_validated ASC LIMIT 50",
			time.Now().Add(-2*time.Hour),
		)
		if err != nil {
			log.Printf("⚠️ [Revalidation] Failed to query stale streams: %v", err)
			return
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var url string
			if err := rows.Scan(&url); err != nil {
				continue
			}
			select {
			case validateCh <- url:
				count++
			default:
				// Channel full, stop feeding
				log.Printf("⚠️ [Revalidation] Channel full, queued %d of available stale streams", count)
				return
			}
		}
		if count > 0 {
			log.Printf("🔄 [Revalidation] Sweep queued %d stale streams for re-validation", count)
		}
	}

	// Run once immediately after the initial delay
	runSweep()

	for range ticker.C {
		runSweep()
	}
}

func validationWorker() {
	for link := range validateCh {
		metricValidationAttempts.Inc()
		validateRDLink(link)

		// THROTTLE: 1.5s between validations to avoid 429s while keeping validation timely
		time.Sleep(1500 * time.Millisecond)
	}
}

// Helper: Probes file size without downloading the video
func probeFileSize(downloadURL string) int64 {
	req, err := http.NewRequest("HEAD", downloadURL, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Stremio")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode == 206 {
		cr := resp.Header.Get("Content-Range") // e.g., bytes 0-0/15000000
		parts := strings.Split(cr, "/")
		if len(parts) == 2 {
			size, _ := strconv.ParseInt(parts[1], 10, 64)
			return size
		}
	} else if resp.StatusCode == 200 {
		size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		return size
	}
	return 0
}

func validateRDLink(targetLink string) {
	finalURL := targetLink

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	// 1. Follow redirects (max 5)
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest("HEAD", finalURL, nil)
		if err != nil {
			markInvalid(targetLink)
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Stremio")

		resp, err := client.Do(req)
		if err != nil {
			markInvalid(targetLink)
			return
		}
		resp.Body.Close()

		// 4xx/5xx means the link is dead - mark invalid immediately
		if resp.StatusCode >= 400 {
			log.Printf("[Validate] ❌ Link returned %d: %s", resp.StatusCode, finalURL)
			markInvalid(targetLink)
			return
		}

		if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
			loc, err := resp.Location()
			if err == nil && loc.String() != "" {
				finalURL = loc.String()
				continue
			}
		}
		break
	}

	downloadURL := finalURL

	// 2. Unrestrict if needed to get the final download URL
	provider := getDebridProviderForHost(finalURL)
	if provider != nil && !strings.Contains(finalURL, ".download.real-debrid.com") {
		dl, err := provider.Unrestrict(finalURL)
		if err == nil && dl != "" {
			downloadURL = dl
		} else {
			markInvalid(targetLink)
			return
		}
	}

	// 3. Range Probe & Size Check the final download URL
	size := probeFileSize(downloadURL)

	// 🛑 "Provider Unavailable" / Error Video Size Check (< 25MB is suspicious)
	if size > 0 && size < 25000000 {
		markInvalid(targetLink)
		return
	}
	if size > 0 {
		markValid(targetLink, size)
		return
	}
	markInvalid(targetLink)
}

func markValid(url string, fileSize int64) {
	metricValidationSuccess.Inc()
	now := time.Now()
	_, err := db.Exec(
		"UPDATE stream_urls SET is_valid = TRUE, fail_count = 0, last_validated = ?, success_count = success_count + 1 WHERE url = ?",
		now, url,
	)
	if err != nil {
		log.Printf("⚠️ Failed to mark stream valid: %v", err)
	}
}

func markInvalid(url string) {
	metricValidationFailed.Inc()
	now := time.Now()
	_, err := db.Exec(
		"UPDATE stream_urls SET is_valid = FALSE, fail_count = fail_count + 1, last_validated = ? WHERE url = ?",
		now, url,
	)
	if err != nil {
		log.Printf("⚠️ Failed to mark stream invalid: %v", err)
	}
}
