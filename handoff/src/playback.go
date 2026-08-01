package main

import (
	"encoding/base64"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func recordStrike(urlStr string) {
	metricStreamFailures.Inc()
	_, err := db.Exec("UPDATE stream_urls SET fail_count = fail_count + 1 WHERE url = ?", urlStr)
	if err == nil {
		log.Printf("[Play] ⚠️ Strike recorded for link: %s", urlStr)
		FireWebhook("stream_failed", "Stream strike recorded for link: "+urlStr)
	}
}

func waitForVFS(targetPath string, maxRetries int) bool {
	rcURL := RcloneRcUrl
	if rcURL == "" {
		rcURL = "http://127.0.0.1:5572"
	}

	log.Printf("[Play] 🔄 Triggering Rclone VFS cache refresh via %s/vfs/refresh?dir=links...", rcURL)
	rcReq, err := http.NewRequest("POST", rcURL+"/vfs/refresh?dir=links", nil)
	if err != nil {
		log.Printf("[Play] ⚠️ Failed to create VFS refresh request: %v", err)
	} else if rcResp, err := httpClient.Do(rcReq); err == nil {
		rcResp.Body.Close()
		log.Printf("[Play] ✅ Rclone VFS refresh command sent successfully.")
	} else {
		log.Printf("[Play] ⚠️ Failed to reach Rclone RC: %v", err)
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		probeReq, err := http.NewRequest("GET", targetPath, nil)
		if err != nil {
			log.Printf("[Play] ⚠️ Failed to create VFS probe request: %v", err)
			continue
		}
		probeReq.Header.Set("Range", "bytes=0-0")
		if RcloneAuth != "" {
			probeReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(RcloneAuth)))
		}

		probeResp, err := httpClient.Do(probeReq)
		if err == nil {
			status := probeResp.StatusCode
			probeResp.Body.Close()
			if status == 200 || status == 206 {
				return true
			}
			log.Printf("[Play] 📉 VFS probe attempt %d/%d returned HTTP %d for %s", attempt, maxRetries, status, targetPath)
		} else {
			log.Printf("[Play] ❌ VFS connection error on attempt %d/%d: %v", attempt, maxRetries, err)
		}

		if attempt < maxRetries {
			time.Sleep(1 * time.Second)
		}
	}
	return false
}

func playHandler(w http.ResponseWriter, r *http.Request, conf Config) {
	targetLink := r.URL.Query().Get("link")
	if targetLink == "" {
		log.Printf("[Play] ❌ Missing link parameter in request")
		http.Error(w, "Missing link", http.StatusBadRequest)
		return
	}

	log.Printf("[Play] 🎬 Play request intercepted: %s", targetLink)
	FireWebhook("stream_started", "Play request started for link: "+targetLink)
	NotifyHomeAssistant("playing", targetLink)

	// Quick DB check: reject obviously dead links immediately
	var failCount int
	var isValid bool
	err := db.QueryRow("SELECT fail_count, is_valid FROM stream_urls WHERE url = ?", targetLink).Scan(&failCount, &isValid)
	if err == nil {
		if failCount >= 3 {
			log.Printf("[Play] ✂️ Blocking stream (3+ strikes): %s", targetLink)
			http.Error(w, "Stream unavailable", http.StatusNotFound)
			return
		}
		if !isValid {
			log.Printf("[Play] ✂️ Blocking stream (invalid): %s", targetLink)
			http.Error(w, "Stream unavailable", http.StatusNotFound)
			return
		}
	}

	finalURL := targetLink
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	log.Printf("[Play] 🕵️ Following redirects to find final video URL...")
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest("GET", finalURL, nil)
		if err != nil {
			log.Printf("[Play] ❌ Failed to create redirect request for %s: %v", finalURL, err)
			recordStrike(targetLink)
			break
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Stremio")

		resp, err := client.Do(req)
		if err != nil {
			recordStrike(targetLink)
			break
		}
		resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
			loc, err := resp.Location()
			if err == nil && loc.String() != "" {
				finalURL = loc.String()
				continue
			}
		}
		// Non-3xx response: we found the final URL or an error status
		break
	}

	// Reject immediately if the final URL returned an error status
	// (re-probe to get the current status)
	reqStatus, err := http.NewRequest("HEAD", finalURL, nil)
	if err == nil {
		reqStatus.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Stremio")
		respStatus, err := client.Do(reqStatus)
		if err == nil && respStatus.StatusCode >= 400 {
			log.Printf("[Play] ❌ Final URL returning error %d for %s", respStatus.StatusCode, finalURL)
			respStatus.Body.Close()
			recordStrike(targetLink)
			http.Error(w, "Stream unavailable", http.StatusNotFound)
			return
		} else if err == nil {
			respStatus.Body.Close()
		}
	}

	// If final URL is junk or suspicious, reject early
	if isSuspiciousURL(finalURL) {
		log.Printf("[Play] ✂️ Suspicious final URL at play time: %s", finalURL)
		http.Error(w, "Blocked suspicious stream", http.StatusNotFound)
		return
	}

	var filename string

	// 🎯 1. DIRECT BYPASS CHECK: .download.real-debrid.com
	if strings.Contains(finalURL, ".download.real-debrid.com") {

		// 🛑 Catch RD "Provider Unavailable" / 8-second video at playback time.
		// Threshold: < 20MB strongly suspected RD error video.
		size := probeFileSize(finalURL)
		if size > 0 && size < 20000000 {
			log.Printf("[Play] ❌ RD Error Video caught at playback (%d bytes)! Striking and rejecting.", size)
			// Mark as invalid in DB
			if _, err := db.Exec("UPDATE stream_urls SET is_valid = FALSE, last_validated = ? WHERE url = ?", time.Now(), targetLink); err != nil {
				log.Printf("⚠️ Failed to update stream validity: %v", err)
			}
			recordStrike(targetLink)
			http.Error(w, "File blocked by Real-Debrid", http.StatusNotFound)
			return
		}

		parsedURL, _ := url.Parse(finalURL)
		filename = filepath.Base(parsedURL.Path)

		rcloneBase := strings.TrimRight(RcloneUrl, "/")
		targetPath := rcloneBase + "/links/" + url.PathEscape(filename)

		log.Printf("[Play] 🛡️ Pre-unrestricted link detected. Probing Rclone VFS: %s", filename)

		if waitForVFS(targetPath, 3) {
			log.Printf("[Play] 🎯 VFS Hit! Streaming via Rclone memory buffers.")
			metricStreamsPlayed.Inc()
			if _, err := db.Exec("UPDATE stream_urls SET success_count = success_count + 1 WHERE url = ?", targetLink); err != nil {
				log.Printf("⚠️ Failed to update success count: %v", err)
			}
			targetUrl, _ := url.Parse(targetPath)
			proxy := &httputil.ReverseProxy{
				Director: func(req *http.Request) {
					req.URL.Scheme = targetUrl.Scheme
					req.URL.Host = targetUrl.Host
					req.URL.Path = targetUrl.Path
					req.URL.RawPath = targetUrl.RawPath
					req.Host = targetUrl.Host
					if RcloneAuth != "" {
						req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(RcloneAuth)))
					}
					req.Header.Del("X-Forwarded-For")
					req.Header.Del("X-Real-Ip")
				},
				BufferPool: proxyPool,
				Transport:  customTransport,
				ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
					log.Printf("[Play] ❌ Rclone proxy error: %v", err)
					recordStrike(targetLink)
					if w.Header().Get("Content-Type") == "" {
						http.Error(w, "Stream error", http.StatusBadGateway)
					}
				},
			}
			proxy.ServeHTTP(w, r)
			return
		}

		log.Printf("[Play] ⚠️ VFS Miss. Engaging Direct RD Proxy.")
		metricStreamsPlayed.Inc()
		if _, err := db.Exec("UPDATE stream_urls SET success_count = success_count + 1 WHERE url = ?", targetLink); err != nil {
			log.Printf("⚠️ Failed to update success count: %v", err)
		}
		targetUrl, _ := url.Parse(finalURL)
		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = targetUrl.Scheme
				req.URL.Host = targetUrl.Host
				req.URL.Path = targetUrl.Path
				req.URL.RawPath = targetUrl.RawPath
				req.Host = targetUrl.Host
				req.Header.Del("X-Forwarded-For")
				req.Header.Del("X-Real-Ip")
			},
			BufferPool: proxyPool,
			Transport:  customTransport,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				log.Printf("[Play] ❌ RD proxy error: %v", err)
				recordStrike(targetLink)
				if w.Header().Get("Content-Type") == "" {
					http.Error(w, "Stream error", http.StatusBadGateway)
				}
			},
		}
		proxy.ServeHTTP(w, r)
		return
	}

	// 🎯 2. STANDARD ROUTE: locked debrid link requiring unrestrict
	if provider := getDebridProviderForHost(finalURL); provider != nil {
		log.Printf("[Play] 🔓 Locked link detected. Un-restricting via %s API...", provider.Name())

		success := false
		var downloadURL string
		for attempt := 1; attempt <= 3; attempt++ {
			if provider.IsRateLimited() {
				break
			}
			dl, err := provider.Unrestrict(finalURL)
			if err == nil && dl != "" {
				downloadURL = dl
				success = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		if !success {
			log.Printf("[Play] ❌ Exhausted unlock attempts for: %s", targetLink)
			recordStrike(targetLink)
			http.Error(w, "Debrid Unrestrict failed", http.StatusBadGateway)
			return
		}

		parsedURL, _ := url.Parse(downloadURL)
		filename = filepath.Base(parsedURL.Path)
	}

	// 🎯 3. Non-Debrid or parse failure: redirect Stremio directly
	if filename == "" || filename == "/" {
		log.Printf("[Play] ⏭️ Non-Debrid link or parse failure. Redirecting Stremio directly.")
		metricStreamsPlayed.Inc()
		w.Header().Set("Location", finalURL)
		w.WriteHeader(http.StatusFound)
		return
	}

	// 🎯 4. RCLONE VFS PROXY FOR NEWLY UNRESTRICTED LINKS
	rcloneBase := strings.TrimRight(RcloneUrl, "/")
	targetPath := rcloneBase + "/links/" + url.PathEscape(filename)

	if !waitForVFS(targetPath, 5) {
		log.Printf("[Play] ⚠️ VFS Miss after retries. Direct proxying might fail.")
	}

	targetUrl, err := url.Parse(targetPath)
	if err != nil {
		recordStrike(targetLink)
		http.Error(w, "Invalid Rclone URL config", http.StatusInternalServerError)
		return
	}

	log.Printf("[Play] 🎯 Streaming newly unlocked video via Rclone VFS: %s", filename)
	if _, err := db.Exec("UPDATE stream_urls SET success_count = success_count + 1 WHERE url = ?", targetLink); err != nil {
		log.Printf("⚠️ Failed to update success count: %v", err)
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetUrl.Scheme
			req.URL.Host = targetUrl.Host
			req.URL.Path = targetUrl.Path
			req.URL.RawPath = targetUrl.RawPath
			req.Host = targetUrl.Host

			if RcloneAuth != "" {
				auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(RcloneAuth))
				req.Header.Set("Authorization", auth)
			}
			req.Header.Del("X-Forwarded-For")
			req.Header.Del("X-Real-Ip")
		},
		BufferPool: proxyPool,
		Transport:  customTransport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[Play] ❌ Final proxy error: %v", err)
			recordStrike(targetLink)
			if w.Header().Get("Content-Type") == "" {
				http.Error(w, "Stream error", http.StatusBadGateway)
			}
		},
	}

	proxy.ServeHTTP(w, r)
}
