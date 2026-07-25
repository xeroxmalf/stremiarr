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
		validateRDLink(link)

		// THROTTLE: 1.5s between validations to avoid 429s while keeping validation timely
		time.Sleep(1500 * time.Millisecond)
	}
}

// Helper: Probes file size without downloading the video
func probeFileSize(downloadURL string) int64 {
	req, _ := http.NewRequest("GET", downloadURL, nil)
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
	log.Printf("[Validation] 🔍 Starting validation for link: %s", targetLink)
	finalURL := targetLink

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	// 1. Follow redirects
	log.Printf("[Validation] 🕵️ Following redirects for: %s", finalURL)
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Stremio")
		resp, err := client.Do(req)

		if err != nil {
			if _, err := db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink); err != nil {
				log.Printf("⚠️ Failed to update stream valid state: %v", err)
			}
			return
		}
		resp.Body.Close()

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
		log.Printf("[Validation] 🔓 Requesting unrestrict from %s for: %s", provider.Name(), finalURL)

		dl, err := provider.Unrestrict(finalURL)
		if err == nil && dl != "" {
			downloadURL = dl
		} else {
			if _, err := db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink); err != nil {
				log.Printf("⚠️ Failed to update stream valid state: %v", err)
			}
			return
		}
	}

	// 3. Range Probe & Size Check the final download URL
	log.Printf("[Validation] 📏 Size probing URL: %s", downloadURL)

	size := probeFileSize(downloadURL)

	// 🛑 "Provider Unavailable" / Error Video Size Check
	if size > 0 && size < 25000000 {
		log.Printf("[Validation] 🚫 Error Video detected (%d bytes). Redacting: %s", size, targetLink)
		if _, err := db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink); err != nil {
			log.Printf("⚠️ Failed to update stream valid state: %v", err)
		}
		return
	} else if size > 0 {
		log.Printf("[Validation] ✅ Range probe successful (%d bytes) for: %s", size, targetLink)
		if _, err := db.Exec("UPDATE stream_urls SET is_valid = TRUE, last_validated = ? WHERE url = ?", time.Now(), targetLink); err != nil {
			log.Printf("⚠️ Failed to update stream valid state: %v", err)
		}
		return
	} else {
		if _, err := db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink); err != nil {
			log.Printf("⚠️ Failed to update stream valid state: %v", err)
		}
		log.Printf("[Validation] 🚫 Range probe failed & redacted silently: %s", targetLink)
		return
	}
}
