package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed ui.html
var uiTemplate string

type Config struct {
	AddonURL       string `json:"addon_url"`
	TorrentioURL   string `json:"torrentio_url"`
	MediafusionURL string `json:"mediafusion_url"`
	StremthruURL   string `json:"stremthru_url"`
}

type AddonSource struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Enabled bool  `json:"enabled"`
}

var (
	addonSources   = []AddonSource{
		{Name: "Comet", URL: "https://comet.feels.legal/manifest.json", Enabled: true},
		{Name: "TorrentIO", URL: "https://torrentio.strem.fun/manifest.json", Enabled: true},
		{Name: "MediaFusion", URL: "https://mediafusion.elfhosted.com/manifest.json", Enabled: true},
		{Name: "StremThru", URL: "https://stremthru.13377001.xyz/manifest.json", Enabled: true},
	}
	sourcesMu      sync.Mutex
)

var AdminPassword = os.Getenv("ADMIN_PASSWORD")
var RcloneUrl = os.Getenv("RCLONE_URL")
var RcloneAuth = os.Getenv("RCLONE_AUTH")
var RcloneRcUrl = os.Getenv("RCLONE_RC_URL")

// --- REAL-DEBRID MULTI-TOKEN POOL ---
var rdApiKeys []string
var rdTokenIdx uint64

func initRdPool() {
	rawKeys := os.Getenv("RD_API_KEY")
	if rawKeys == "" {
		log.Fatal("❌ ERROR: RD_API_KEY is missing!")
	}
	
	for _, k := range strings.Split(rawKeys, ",") {
		trimmed := strings.TrimSpace(k)
		if trimmed != "" {
			rdApiKeys = append(rdApiKeys, trimmed)
		}
	}

	if len(rdApiKeys) == 0 {
		log.Fatal("❌ ERROR: No valid Real-Debrid tokens found in RD_API_KEY!")
	}

	log.Printf("🔑 Initialized Real-Debrid token pool with %d keys", len(rdApiKeys))
}

func getRdApiKey() string {
	idx := atomic.AddUint64(&rdTokenIdx, 1) % uint64(len(rdApiKeys))
	return rdApiKeys[idx]
}

var startTime = time.Now()

// --- ZERO-STUTTER BUFFER POOL ---
type bytesPool struct {
	pool sync.Pool
}

func (p *bytesPool) Get() []byte {
	if buf := p.pool.Get(); buf != nil {
		return buf.([]byte)
	}
	return make([]byte, 128*1024)
}

func (p *bytesPool) Put(buf []byte) {
	p.pool.Put(buf)
}

var proxyPool = &bytesPool{}

// --- CATALOG CACHING ---
type cacheEntry struct {
	Body       []byte
	Headers    http.Header
	StatusCode int
	ExpiresAt  time.Time
}

var catalogCache sync.Map

// --- ALIAS MAPPINGS ---
var (
	aliasMappings = make(map[string]Config)
	mappingsMutex sync.RWMutex
	mappingsPath  = "/data/mappings.json"
)

// --- SQLITE DB CORE ---
var db *sql.DB

func initDB() {
	os.MkdirAll("/data", 0755)
	var err error
	// Optimized SQLite settings for higher throughput and concurrency
	db, err = sql.Open("sqlite3", "/data/streams.db?_busy_timeout=5000&_journal_mode=WAL&_sync=NORMAL&_cache_size=-20000")
	if err != nil {
		log.Fatalf("❌ Failed to open SQLite DB: %v", err)
	}

	// Performance tuning: reduce IO by using memory for temp store
	_, err = db.Exec("PRAGMA temp_store = MEMORY;")
	if err != nil {
		log.Printf("⚠️ Warning: Failed to set temp_store PRAGMA: %v", err)
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS stream_cache (
            request_id TEXT PRIMARY KEY,
            streams_json TEXT,
            updated_at DATETIME
        );
        CREATE TABLE IF NOT EXISTS stream_urls (
            url TEXT PRIMARY KEY,
            is_valid BOOLEAN DEFAULT 1,
            fail_count INTEGER DEFAULT 0,
            last_validated DATETIME
        );
    `)
	if err != nil {
		log.Fatalf("❌ Failed to init SQLite tables: %v", err)
	}

	log.Printf("💾 SQLite database initialized at /data/streams.db")
}

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
	if err != nil { return 0 }
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
			db.Exec("UPDATE stream_urls SET is_valid = 0, last_validated = ? WHERE url = ?", time.Now(), targetLink)
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
	if strings.Contains(finalURL, "real-debrid.com") && !strings.Contains(finalURL, ".download.real-debrid.com") {
		log.Printf("[Validation] 🔓 Requesting unrestrict from RD API for: %s", finalURL)
		apiURL := "https://api.real-debrid.com/rest/1.0/unrestrict/link"
		payload := "link=" + url.QueryEscape(finalURL)

		req, _ := http.NewRequest("POST", apiURL, strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+getRdApiKey())
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err == nil {
			if resp.StatusCode == 200 {
				var rdResp struct {
					Download string `json:"download"`
				}
				err = json.NewDecoder(resp.Body).Decode(&rdResp)
				resp.Body.Close()
				if err == nil && rdResp.Download != "" {
					downloadURL = rdResp.Download
				} else {
					db.Exec("UPDATE stream_urls SET is_valid = 0, last_validated = ? WHERE url = ?", time.Now(), targetLink)
					return
				}
			} else if resp.StatusCode == 404 || resp.StatusCode == 403 {
				resp.Body.Close()
				db.Exec("UPDATE stream_urls SET is_valid = 0, last_validated = ? WHERE url = ?", time.Now(), targetLink)
				log.Printf("[Validation] 🚫 API Dead link flagged & redacted silently: %s", targetLink)
				return
			} else {
				resp.Body.Close()
				db.Exec("UPDATE stream_urls SET last_validated = ? WHERE url = ?", time.Now(), targetLink)
				return
			}
		} else {
			db.Exec("UPDATE stream_urls SET last_validated = ? WHERE url = ?", time.Now(), targetLink)
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
			db.Exec("UPDATE stream_urls SET is_valid = 0, last_validated = ? WHERE url = ?", time.Now(), targetLink)
			return
		} else if size > 0 {
			log.Printf("[Validation] ✅ Range probe successful (%d bytes) for: %s", size, targetLink)
			db.Exec("UPDATE stream_urls SET is_valid = 1, last_validated = ? WHERE url = ?", time.Now(), targetLink)
			return
		} else {
			db.Exec("UPDATE stream_urls SET is_valid = 0, last_validated = ? WHERE url = ?", time.Now(), targetLink)
			log.Printf("[Validation] 🚫 Range probe failed & redacted silently: %s", targetLink)
			return
		}
	}

	log.Printf("[Validation] ✅ Validation complete and successful for: %s", targetLink)
	db.Exec("UPDATE stream_urls SET is_valid = 1, last_validated = ? WHERE url = ?", time.Now(), targetLink)
}

func recordStrike(urlStr string) {
	_, err := db.Exec("UPDATE stream_urls SET fail_count = fail_count + 1 WHERE url = ?", urlStr)
	if err == nil {
		log.Printf("[Play] ⚠️ Strike recorded for link: %s", urlStr)
	}
}

// --------------------------------

func loadMappings() {
	mappingsMutex.Lock()
	defer mappingsMutex.Unlock()

	if _, err := os.Stat(mappingsPath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(mappingsPath)
	if err != nil {
		log.Printf("⚠️ Failed to read mappings.json: %v", err)
		return
	}

	if err := json.Unmarshal(data, &aliasMappings); err != nil {
		log.Printf("⚠️ Failed to parse mappings.json: %v", err)
	} else {
		log.Printf("📁 Loaded %d alias mappings from disk", len(aliasMappings))
	}
}

func saveMappings() {
	mappingsMutex.RLock()
	data, err := json.MarshalIndent(aliasMappings, "", "  ")
	mappingsMutex.RUnlock()

	if err != nil {
		log.Printf("❌ Failed to serialize mappings: %v", err)
		return
	}

	os.MkdirAll("/data", 0755)

	if err := os.WriteFile(mappingsPath, data, 0644); err != nil {
		log.Printf("❌ Failed to write mappings.json: %v", err)
	}
}

func generateID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

var customTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ForceAttemptHTTP2:     false,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   32,
	MaxConnsPerHost:       32,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ReadBufferSize:        64 * 1024,
}

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	},
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime)

	initDB()
	initRdPool()
	initValidationPool()

	if AdminPassword == "" {
		AdminPassword = "admin"
	}
	if RcloneUrl == "" {
		log.Fatal("❌ ERROR: RCLONE_URL is missing!")
	}

	loadMappings()

	http.HandleFunc("/", routeHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 DebridHandoff Engine Initialized!")
	log.Printf("📡 Listening for Stremio traffic on port %s", port)

	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
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

func routeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Public health endpoint for Docker healthchecks (no auth required)
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		return
	}

	// Dashboard endpoint (authenticated)
	if r.URL.Path == "/dashboard" {
		if !requireAuth(w, r) {
			return
		}
		serveDashboard(w, r)
		return
	}

	// Web UI and API endpoints (authenticated)
	if r.URL.Path == "/ui" || r.URL.Path == "/ui/" {
		serveWebUI(w, r)
		return
	}

	// API endpoints
	if strings.HasPrefix(r.URL.Path, "/api/") {
		serveAPI(w, r)
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	if path == "" || path == "generate" {
		uiHandler(w, r)
		return
	}

	idOrConfig := parts[0]
	var conf Config

	mappingsMutex.RLock()
	mappedConf, exists := aliasMappings[idOrConfig]
	mappingsMutex.RUnlock()

	if exists {
		conf = mappedConf
	} else {
		configData, err := base64.RawURLEncoding.DecodeString(idOrConfig)
		if err != nil {
			log.Printf("[Router] ⚠️ Unknown alias or invalid config format requested: %s", idOrConfig)
			http.Error(w, "Invalid Configuration format or Unknown Alias", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(configData, &conf); err != nil {
			log.Printf("[Router] ⚠️ Failed to parse configuration from base64")
			http.Error(w, "Failed to parse configuration", http.StatusBadRequest)
			return
		}
	}

	action := parts[1]
	if action == "play" {
		playHandler(w, r, conf)
	} else if action == "stream" {
		streamHandler(w, r, conf, idOrConfig, strings.Join(parts[1:], "/"), r.URL.RawQuery)
	} else if action == "manifest.json" {
		manifestHandler(w, r, conf, strings.Join(parts[1:], "/"), r.URL.RawQuery)
	} else {
		proxyRequest(w, r, conf.AddonURL, strings.Join(parts[1:], "/"), r.URL.RawQuery)
	}
}

// --------------------------------
// ENHANCED STREAM ROUTER (SWR CACHING)
// --------------------------------
func streamHandler(w http.ResponseWriter, r *http.Request, conf Config, idOrConfig string, subPath string, rawQuery string) {
	targetURL := getTargetURL(conf.AddonURL, subPath, rawQuery)
	log.Printf("[Stream] 🔍 Requesting streams for: %s", targetURL)

	var cachedStreams string
	err := db.QueryRow("SELECT streams_json FROM stream_cache WHERE request_id = ?", targetURL).Scan(&cachedStreams)

	// 1. Optimistic SWR Serve
	if err == nil && cachedStreams != "" {
		log.Printf("[Stream] ⚡ Serving Optimistic Cache for: %s", targetURL)
		serveStreamsJSON(w, r, []byte(cachedStreams), idOrConfig)

		// Fire Stale-While-Revalidate worker into background
		go func() {
			fetchAndCacheStreams(targetURL, conf)
		}()
		return
	}

	// 2. Cache Miss Sync Fetch
	body, err := fetchAndCacheStreams(targetURL, conf)
	if err != nil {
		log.Printf("[Stream] ❌ Failed to fetch streams: %v", err)
		http.Error(w, "Failed to fetch streams", http.StatusBadGateway)
		return
	}

	serveStreamsJSON(w, r, body, idOrConfig)
}

type StreamFetcher struct {
	Streams []interface{}
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

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
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
	for _, s := range allFetcher.Streams {
		if stream, ok := s.(map[string]interface{}); ok {
			if urlStr, ok := stream["url"].(string); ok {
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
				db.Exec("INSERT OR IGNORE INTO stream_urls (url, is_valid, fail_count) VALUES (?, 1, 0)", urlStr)
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
	db.Exec("INSERT OR REPLACE INTO stream_cache (request_id, streams_json, updated_at) VALUES (?, ?, ?)", targetURL, string(body), time.Now())

	log.Printf("[Stream] ✅ Fetched %d streams from %d sources", len(deduped), len(sources))
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
				// No usable URL; drop silently
				redactedCount++
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
			var isValid bool
			var lastValidated time.Time
			err := db.QueryRow("SELECT fail_count, is_valid, last_validated FROM stream_urls WHERE url = ?", urlStr).Scan(&failCount, &isValid, &lastValidated)

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

			wrappedCount++
			finalStreams = append(finalStreams, stream)
		}

		result["streams"] = finalStreams
	}

	modifiedBody, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")
	w.Write(modifiedBody)
	log.Printf("[Stream] ✅ Served %d streams (%d actively redacted, incl. junk/dead/dupes)", wrappedCount, redactedCount)
}

// --------------------------------
// PLAY FAILOVER LOGIC
// --------------------------------

func waitForVFS(targetPath string, maxRetries int) bool {
	rcURL := RcloneRcUrl
	if rcURL == "" {
		rcURL = "http://127.0.0.1:5572"
	}

	log.Printf("[Play] 🔄 Triggering Rclone VFS cache refresh via %s/vfs/refresh?dir=links...", rcURL)
	rcReq, _ := http.NewRequest("POST", rcURL+"/vfs/refresh?dir=links", nil)
	if rcResp, err := httpClient.Do(rcReq); err == nil {
		rcResp.Body.Close()
		log.Printf("[Play] ✅ Rclone VFS refresh command sent successfully.")
	} else {
		log.Printf("[Play] ⚠️ Failed to reach Rclone RC: %v", err)
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		probeReq, _ := http.NewRequest("GET", targetPath, nil)
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
		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Stremio")

		resp, err := client.Do(req)
		if err != nil {
			recordStrike(targetLink)
			break
		}
		resp.Body.Close()

		// Hard reject 4xx/5xx non-redirect responses
		if resp.StatusCode >= 400 {
			log.Printf("[Play] ❌ Non-redirect error during redirect follow: %d for %s", resp.StatusCode, finalURL)
			recordStrike(targetLink)
			break
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
			db.Exec("UPDATE stream_urls SET is_valid = 0, last_validated = ? WHERE url = ?", time.Now(), targetLink)
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
			}
			proxy.ServeHTTP(w, r)
			return
		}

		log.Printf("[Play] ⚠️ VFS Miss. Engaging Direct RD Proxy.")
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
		}
		proxy.ServeHTTP(w, r)
		return

	// 🎯 2. STANDARD ROUTE: real-debrid.com links
	} else if strings.Contains(finalURL, "real-debrid.com") {
		log.Printf("[Play] 🔓 Locked RD link detected. Un-restricting via API...")
		apiURL := "https://api.real-debrid.com/rest/1.0/unrestrict/link"
		payload := "link=" + url.QueryEscape(finalURL)

		success := false
		for attempt := 1; attempt <= 3; attempt++ {
			req, _ := http.NewRequest("POST", apiURL, strings.NewReader(payload))
			req.Header.Set("Authorization", "Bearer "+getRdApiKey())
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, err := httpClient.Do(req)
			if err == nil {
				if resp.StatusCode == 200 {
					var rdResp struct {
						Filename string `json:"filename"`
						Download string `json:"download"`
					}
					json.NewDecoder(resp.Body).Decode(&rdResp)
					resp.Body.Close()

					if rdResp.Filename != "" && rdResp.Filename != "/" {
						filename = rdResp.Filename
						log.Printf("[Play] ✅ Successfully unlocked: %s", filename)
						success = true
						break
					}

					log.Printf("[Play] ⚠️ RD unrestrict returned no filename. Link likely dead.")
					recordStrike(targetLink)
					db.Exec("UPDATE stream_urls SET is_valid = 0, last_validated = ? WHERE url = ?", time.Now(), targetLink)
					break
				}

				resp.Body.Close()
				if resp.StatusCode == 404 || resp.StatusCode == 403 {
					log.Printf("[Play] ❌ RD API dead link (status=%d). Marking invalid.", resp.StatusCode)
					recordStrike(targetLink)
					db.Exec("UPDATE stream_urls SET is_valid = 0, last_validated = ? WHERE url = ?", time.Now(), targetLink)
					break
				}

				if resp.StatusCode == 503 || resp.StatusCode == 429 {
					log.Printf("[Play] ⚠️ RD API returned %d. Retrying (%d/3)...", resp.StatusCode, attempt)
					time.Sleep(500 * time.Millisecond)
					continue
				}

				log.Printf("[Play] ❌ RD API failed with status: %d", resp.StatusCode)
			}
			break
		}

		if !success {
			log.Printf("[Play] ✂️ RD unrestrict failed after retries for: %s", targetLink)
		}
	}

	// 🎯 3. FALLBACK for non-RD or parse failures
	if filename == "" || filename == "/" {
		log.Printf("[Play] ⏭️ Non-RD link or parse failure. Bypassing proxy and redirecting Stremio.")
		recordStrike(targetLink)
		http.Redirect(w, r, finalURL, http.StatusFound)
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
	}

	proxy.ServeHTTP(w, r)
}

func manifestHandler(w http.ResponseWriter, r *http.Request, conf Config, subPath string, rawQuery string) {
	targetURL := getTargetURL(conf.AddonURL, subPath, rawQuery)
	log.Printf("[Manifest] 📦 Intercepting manifest from: %s", conf.AddonURL)

	resp, err := httpClient.Get(targetURL)
	if err != nil {
		log.Printf("[Manifest] ❌ Failed to fetch manifest: %v", err)
		http.Error(w, "Failed to fetch manifest", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var manifest map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&manifest)

	if name, ok := manifest["name"].(string); ok {
		manifest["name"] = name + " (Handoff)"
	}
	if id, ok := manifest["id"].(string); ok {
		manifest["id"] = id + ".handoff"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(manifest)
	log.Printf("[Manifest] ✅ Successfully wrapped and injected manifest identity.")
}

func proxyRequest(w http.ResponseWriter, r *http.Request, addonURL string, subPath string, rawQuery string) {
	targetURL := getTargetURL(addonURL, subPath, rawQuery)

	if entry, ok := catalogCache.Load(targetURL); ok {
		ce := entry.(cacheEntry)
		if time.Now().Before(ce.ExpiresAt) {
			for k, v := range ce.Headers {
				w.Header()[k] = v
			}
			w.WriteHeader(ce.StatusCode)
			w.Write(ce.Body)
			log.Printf("[Catalog] ⚡ Cache hit for: %s", targetURL)
			return
		}
		catalogCache.Delete(targetURL)
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

	catalogCache.Store(targetURL, cacheEntry{
		Body:       body,
		Headers:    resp.Header.Clone(),
		StatusCode: resp.StatusCode,
		ExpiresAt:  time.Now().Add(5 * time.Minute),
	})

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	u, p, ok := r.BasicAuth()
	if !ok || u != "admin" || p != AdminPassword {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized.\n"))
		return false
	}
	return true
}

func serveAPI(w http.ResponseWriter, r *http.Request) {
	// Require authentication for all API endpoints
	if !requireAuth(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path
	switch {
	case path == "/api/stats":
		serveAPIStats(w, r)
	case path == "/api/sources" && r.Method == "GET":
		serveAPISourcesGet(w, r)
	case path == "/api/sources" && r.Method == "PUT":
		serveAPISourcesUpdate(w, r)
	case path == "/api/mappings":
		serveAPIMappings(w, r)
	case path == "/api/clean":
		serveAPIClean(w, r)
	case path == "/api/cache":
		serveAPICache(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func serveAPIStats(w http.ResponseWriter, r *http.Request) {
	var totalURLs, validURLs, failedURLs int
	var cachedRequests, recentValidations int

	db.QueryRow("SELECT COUNT(*) FROM stream_urls").Scan(&totalURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE is_valid = 1").Scan(&validURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE fail_count > 0").Scan(&failedURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_cache").Scan(&cachedRequests)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE last_validated > ?", time.Now().Add(-10*time.Minute)).Scan(&recentValidations)

	w.Write([]byte(fmt.Sprintf(`{
		"totalStreams": %d,
		"validStreams": %d,
		"failedStreams": %d,
		"cachedRequests": %d,
		"recentValidations": %d,
		"uptime": "%s"
	}`,
		totalURLs, validURLs, failedURLs, cachedRequests, recentValidations,
		time.Since(startTime).String(),
	)))
}

func serveAPISourcesGet(w http.ResponseWriter, r *http.Request) {
	sourcesMu.Lock()
	sources := make([]AddonSource, len(addonSources))
	copy(sources, addonSources)
	sourcesMu.Unlock()

	w.Write([]byte(fmt.Sprintf(`{"sources": %s}`, marshalJSON(sources))))
}

func serveAPISourcesUpdate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		Sources []AddonSource `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	sourcesMu.Lock()
	addonSources = req.Sources
	sourcesMu.Unlock()

	w.Write([]byte(`{"status": "ok", "message": "Sources updated"}`))
}

func serveAPIMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "DELETE" {
		r.ParseForm()
		alias := r.FormValue("alias")
		if alias != "" {
			mappingsMutex.Lock()
			delete(aliasMappings, alias)
			mappingsMutex.Unlock()
			saveMappings()
		}
	}

	mappingsMutex.RLock()
	mappings := make(map[string]Config, len(aliasMappings))
	for k, v := range aliasMappings {
		mappings[k] = v
	}
	mappingsMutex.RUnlock()

	w.Write([]byte(fmt.Sprintf(`{"mappings": %s}`, marshalJSON(mappings))))
}

func serveAPIClean(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Delete streams with 3+ failures
	_, err := db.Exec("DELETE FROM stream_urls WHERE fail_count >= 3")
	if err != nil {
		http.Error(w, "Failed to clean streams", http.StatusInternalServerError)
		return
	}

	// Update invalid streams
	_, err = db.Exec("UPDATE stream_urls SET is_valid = 0 WHERE last_validated < ?", time.Now().Add(-2*time.Hour))
	if err != nil {
		http.Error(w, "Failed to update invalid streams", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(`{"status": "ok", "message": "Cleaned failed and stale streams"}`))
}

func serveAPICache(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear all cached streams
	_, err := db.Exec("DELETE FROM stream_cache")
	if err != nil {
		http.Error(w, "Failed to clear cache", http.StatusInternalServerError)
		return
	}

	// Clear in-memory cache
	catalogCache.Clear()

	w.Write([]byte(`{"status": "ok", "message": "Cache cleared"}`))
}

func marshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func serveWebUI(w http.ResponseWriter, r *http.Request) {
	if !requireAuth(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(uiTemplate))
}

func serveDashboard(w http.ResponseWriter, r *http.Request) {
	// Simple, read-only, public dashboard
	// Shows real-time stats for streaming performance

	var totalURLs int
	var validURLs int
	var failedURLs int
	db.QueryRow("SELECT COUNT(*) FROM stream_urls").Scan(&totalURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE is_valid = 1").Scan(&validURLs)
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE fail_count > 0").Scan(&failedURLs)

	// Count cached streams
	var cachedRequests int
	db.QueryRow("SELECT COUNT(*) FROM stream_cache").Scan(&cachedRequests)

	// Recent validation activity (last 10 minutes)
	var recentValidations int
	db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE last_validated > ?", time.Now().Add(-10*time.Minute)).Scan(&recentValidations)

	// System uptime (simple)
	uptime := time.Since(startTime)

	// Generate HTML dashboard
	html := fmt.Sprintf(`
<html>
<head>
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>DebridHandoff Dashboard</title>
	<style>
		body {
			margin: 0;
			padding: 20px;
			background: #111;
			color: #fff;
			font-family: sans-serif;
			display: flex;
			justify-content: center;
		}
		.container {
			max-width: 800px;
			width: 100%%;
		}
		h1 {
			text-align: center;
			color: #00adb5;
		}
		.stats {
			display: grid;
			grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
			gap: 15px;
			margin-top: 20px;
		}
		.card {
			background: #222;
			padding: 15px;
			border-radius: 8px;
			box-shadow: 0 4px 10px rgba(0,0,0,0.5);
		}
		.label {
			color: #aaa;
			font-size: 0.9em;
			margin-bottom: 5px;
		}
		.value {
			color: #00adb5;
			font-size: 1.2em;
			font-weight: bold;
		}
		.actions {
			margin-top: 20px;
			display: flex;
			justify-content: space-between;
			gap: 10px;
		}
		.button {
			padding: 10px 15px;
			background: #00adb5;
			color: #111;
			border: none;
			border-radius: 5px;
			cursor: pointer;
			font-weight: bold;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>DebridHandoff Dashboard</h1>
		<p style="text-align:center; color:#aaa;">Real-time stats for streaming performance</p>

		<div class="stats">
			<div class="card">
				<div class="label">Total Streams</div>
				<div class="value">%d</div>
			</div>
			<div class="card">
				<div class="label">Valid Streams</div>
				<div class="value">%d</div>
			</div>
			<div class="card">
				<div class="label">Failed Streams</div>
				<div class="value">%d</div>
			</div>
			<div class="card">
				<div class="label">Cached Requests</div>
				<div class="value">%d</div>
			</div>
			<div class="card">
				<div class="label">Recent Validations</div>
				<div class="value">%d</div>
			</div>
			<div class="card">
				<div class="label">Uptime</div>
				<div class="value">%s</div>
			</div>
		</div>

		<div class="actions">
			<button class="button" onclick="window.location.reload()">Refresh</button>
			<button class="button" onclick="window.location.href='/health'">Health Check</button>
		</div>
	</div>
</body>
</html>
	`,
		totalURLs,
		validURLs,
		failedURLs,
		cachedRequests,
		recentValidations,
		uptime.String(),
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func uiHandler(w http.ResponseWriter, r *http.Request) {
	u, p, ok := r.BasicAuth()
	if !ok || u != "admin" || p != AdminPassword {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized.\n"))
		return
	}

	host := "http://" + r.Host
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		host = "https://" + r.Host
	}

	if r.Method == "POST" {
		r.ParseForm()
		urls := strings.Split(r.FormValue("addon_urls"), "\n")
		alias := strings.TrimSpace(r.FormValue("alias"))

		var resultsHTML string
		successCount := 0

		for _, u := range urls {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}

			conf := Config{AddonURL: u}

			var linkID string
			if alias != "" {
				linkID = alias
			} else {
				linkID = generateID()
			}

			mappingsMutex.Lock()
			aliasMappings[linkID] = conf
			mappingsMutex.Unlock()
			saveMappings()

			finalManifest := fmt.Sprintf("%s/%s/manifest.json", host, linkID)
			resultsHTML += fmt.Sprintf(`
				<div style="margin-bottom: 15px;">
					<input type="text" value="%s" style="width:100%%; padding:12px; border-radius:5px; border: 1px solid #444; background: #222; color: #00adb5; font-family: monospace; font-size: 1.1em;" readonly onclick="this.select();">
				</div>`, finalManifest)
			successCount++
		}

		log.Printf("[UI] 🛠️ Generated %d secure wrapper links via the Web Panel", successCount)

		html := fmt.Sprintf(`<html><body style="background:#111; color:white; font-family:sans-serif; text-align:center; padding: 50px; max-width: 800px; margin: 0 auto;">
			<h2>✅ Wrapped Successfully!</h2>
			<p style="color: #aaa; margin-bottom: 30px;">Click any link below to highlight it, then copy and paste into Stremio's search bar.</p>
			%s
			<br><br><a href="/" style="color:#00adb5; text-decoration: none; font-size: 1.1em; font-weight: bold;">🔙 Wrap More Addons</a>
		</body></html>`, resultsHTML)

		w.Write([]byte(html))
		return
	}

	log.Printf("[UI] 🖥️ Web Panel accessed")

	mappingsMutex.RLock()
	var existingList string
	for id, conf := range aliasMappings {
		url := fmt.Sprintf("%s/%s/manifest.json", host, id)
		existingList += fmt.Sprintf(`
			<div style="margin-bottom: 10px; background: #333; padding: 10px; border-radius: 5px; text-align: left;">
				<div style="color: #00adb5; font-weight: bold; margin-bottom: 5px;">Alias: %s</div>
				<div style="color: #aaa; font-size: 0.85em; margin-bottom: 5px; word-wrap: break-word;">Target: %s</div>
				<input type="text" value="%s" style="width:100%%; padding:8px; border-radius:3px; border: 1px solid #555; background: #222; color: #fff; font-family: monospace; font-size: 0.9em;" readonly onclick="this.select();">
			</div>`, id, conf.AddonURL, url)
	}
	mappingsMutex.RUnlock()

	w.Write([]byte(fmt.Sprintf(`<html><body style="background:#111; color:white; font-family:sans-serif; padding: 50px;">
		<h2 style="text-align:center; color:#00adb5;">🚀 DebridHandoff Wrapper</h2>
		
		<form method="POST" style="max-width: 600px; margin: 0 auto; background: #222; padding: 30px; border-radius: 10px; box-shadow: 0 4px 15px rgba(0,0,0,0.5); margin-bottom: 30px;">
			<label style="font-size: 1.1em; font-weight: bold;">Target Addon Manifest URL:</label>
			<p style="color: #aaa; font-size: 0.9em; margin-bottom: 15px;">Paste your configured addon URL below.</p>
			<textarea name="addon_urls" required placeholder="https://comet.defnotmy.site/manifest.json" style="width:100%%; height: 80px; padding:15px; margin-bottom:15px; border-radius:5px; border:1px solid #444; background:#333; color:white; font-family: monospace; resize: vertical;"></textarea>
			
			<label style="font-size: 1.1em; font-weight: bold;">Short Alias (Optional):</label>
			<p style="color: #aaa; font-size: 0.9em; margin-bottom: 15px;">Provide a steady short name (e.g. 'comet'). If left blank, a random ID is generated.</p>
			<input type="text" name="alias" placeholder="e.g. comet" style="width:100%%; padding:15px; margin-bottom:20px; border-radius:5px; border:1px solid #444; background:#333; color:white; font-family: monospace;">
			
			<input type="submit" value="Generate Secure Link" style="width:100%%; padding:15px; background:#00adb5; color:white; border:none; border-radius:5px; cursor:pointer; font-weight:bold; font-size: 1.1em; transition: background 0.2s;">
		</form>

		<div style="max-width: 600px; margin: 0 auto;">
			<h3 style="font-size: 1.2em; border-bottom: 1px solid #444; padding-bottom: 10px; margin-bottom: 15px;">Existing Mappings</h3>
			%s
		</div>
	</body></html>`, existingList)))
}
