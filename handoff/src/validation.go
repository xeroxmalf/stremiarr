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
	log.Printf("🛡️ API Validation Pool initialized (2 concurrent workers with rate limits)")
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
			db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink)
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
			db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink)
			return
		}
	}

	// 3. Range Probe & Size Check the .download.real-debrid.com URL
	if strings.Contains(downloadURL, ".download.real-debrid.com") {
		log.Printf("[Validation] 📏 Size probing URL: %s", downloadURL)

		size := probeFileSize(downloadURL)

		// 🛑 RD "Provider Unavailable" Video Size Check
		if size > 0 && size < 25000000 {
			log.Printf("[Validation] 🚫 RD Error Video detected (%d bytes). Redacting: %s", size, targetLink)
			db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink)
			return
		} else if size > 0 {
			log.Printf("[Validation] ✅ Range probe successful (%d bytes) for: %s", size, targetLink)
			db.Exec("UPDATE stream_urls SET is_valid = TRUE, last_validated = ? WHERE url = ?", time.Now(), targetLink)
			return
		} else {
			db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink)
			log.Printf("[Validation] 🚫 Range probe failed & redacted silently: %s", targetLink)
			return
		}
	}

	log.Printf("[Validation] ✅ Validation complete and successful for: %s", targetLink)
	db.Exec("UPDATE stream_urls SET is_valid = TRUE, last_validated = ? WHERE url = ?", time.Now(), targetLink)
}
