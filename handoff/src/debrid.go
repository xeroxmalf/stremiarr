package main


import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// --- ZURG RD LOGIC ---
func getBackoffDuration(attempt int) time.Duration {
	base := 1
	maxDuration := 60
	backoff := base * int(math.Pow(2, float64(attempt)))
	if backoff > maxDuration {
		backoff = maxDuration
	}

	maxJitter := float64(backoff) * 0.2
	jitter := rand.Float64() * maxJitter

	finalDuration := float64(backoff) + jitter
	return time.Duration(finalDuration) * time.Second
}

func rdDo(req *http.Request) (*http.Response, error) {
	attempt := 0
	for {
		// Clone request body if it exists so we can retry
		var bodyBytes []byte
		if req.Body != nil {
			bodyBytes, _ = io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := httpClient.Do(req)

		// Reset req.Body for next iteration if needed
		if req.Body != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		if err != nil && strings.Contains(err.Error(), "context canceled") {
			return nil, err
		}

		isNetworkError := err != nil && (strings.Contains(err.Error(), "timeout") ||
			strings.Contains(err.Error(), "EOF") ||
			strings.Contains(err.Error(), "connection reset by peer") ||
			strings.Contains(err.Error(), "broken pipe"))

		if isNetworkError {
			if attempt >= 10 { // Max 10 retries for network
				return nil, err
			}
			secs := getBackoffDuration(attempt)
			log.Printf("⚠️ [RD] Network error: %v, retrying in %v...", err, secs)
			time.Sleep(secs)
			attempt++
			continue
		}

		if err != nil {
			return nil, err
		}

		// Read response body so we can inspect JSON errors and also recreate it
		respBodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			// Try parsing as RD JSON error
			var apiErr struct {
				Error     string `json:"error"`
				ErrorCode int    `json:"error_code"`
			}
			_ = json.Unmarshal(respBodyBytes, &apiErr)

			retry := false
			var wait time.Duration

			if apiErr.ErrorCode == 5 || apiErr.ErrorCode == 34 || apiErr.ErrorCode == 36 || apiErr.ErrorCode == -1 || resp.StatusCode == 429 || resp.StatusCode == 503 {
				retry = true
				wait = getBackoffDuration(attempt)
				log.Printf("⚠️ [RD] Rate Limit / Server Error (Code: %d, HTTP: %d), retrying in %v...", apiErr.ErrorCode, resp.StatusCode, wait)
			} else if apiErr.ErrorCode == 23 { // traffic_exhausted
				retry = true
				wait = 15 * time.Second
				log.Printf("⚠️ [RD] Traffic exhausted! Go to RD settings and uncheck 'Use my Remote Traffic automatically when needed'. Retrying in %v...", wait)
			}

			if retry {
				time.Sleep(wait)
				attempt++
				continue
			}
		}

		// Success or non-retriable error
		resp.Body = io.NopCloser(bytes.NewReader(respBodyBytes))
		return resp, nil
	}
}

type unrestrictCacheEntry struct {
	Link      string
	ExpiresAt time.Time
}

var globalUnrestrictCache sync.Map


type DebridProvider interface {
	Name() string
	Hosts() []string
	Unrestrict(link string) (string, error)
	IsRateLimited() bool
}

// --- REAL-DEBRID IMPLEMENTATION ---
type RdKey struct {
	Token          string
	LockedOutUntil time.Time
}

type RealDebridProvider struct {
	Keys     []*RdKey
	TokenIdx uint64
	Mu       sync.Mutex
}

func (rd *RealDebridProvider) Name() string {
	return "Real-Debrid"
}

func (rd *RealDebridProvider) Hosts() []string {
	return []string{"real-debrid.com"}
}

func (rd *RealDebridProvider) getActiveToken() string {
	rd.Mu.Lock()
	defer rd.Mu.Unlock()
	if len(rd.Keys) == 0 {
		return ""
	}
	now := time.Now()
	for i := 0; i < len(rd.Keys); i++ {
		idx := (atomic.AddUint64(&rd.TokenIdx, 1)) % uint64(len(rd.Keys))
		if rd.Keys[idx].LockedOutUntil.Before(now) {
			return rd.Keys[idx].Token
		}
	}
	// Fallback
	idx := (atomic.AddUint64(&rd.TokenIdx, 1)) % uint64(len(rd.Keys))
	return rd.Keys[idx].Token
}

func (rd *RealDebridProvider) reportError(token string, statusCode int) {
	if statusCode == 429 || statusCode == 503 {
		rd.Mu.Lock()
		defer rd.Mu.Unlock()
		for _, key := range rd.Keys {
			if key.Token == token {
				key.LockedOutUntil = time.Now().Add(5 * time.Minute)
				displayToken := token
				if len(token) > 4 {
					displayToken = "..." + token[len(token)-4:]
				}
				log.Printf("🔑 [Rate Limit] Locking out RD Key (%s) for 5 mins", displayToken)
				break
			}
		}
	}
}

func (rd *RealDebridProvider) Unrestrict(link string) (string, error) {
	token := rd.getActiveToken()
	if token == "" {
		return "", errors.New("no RD keys available")
	}

	apiURL := "https://api.real-debrid.com/rest/1.0/unrestrict/link"
	payload := "link=" + url.QueryEscape(link)

	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(payload))
	// Check Cache First
	if cached, ok := globalUnrestrictCache.Load(link); ok {
		entry := cached.(unrestrictCacheEntry)
		if time.Now().Before(entry.ExpiresAt) {
			return entry.Link, nil
		}
		globalUnrestrictCache.Delete(link)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := rdDo(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 || resp.StatusCode == 503 {
		rd.reportError(token, resp.StatusCode)
		return "", errors.New("rate limited")
	}

	if resp.StatusCode == 200 {
		var rdResp struct {
			Download string `json:"download"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&rdResp); err == nil && rdResp.Download != "" {
			// Cache for 4 hours like Zurg
			globalUnrestrictCache.Store(link, unrestrictCacheEntry{
				Link:      rdResp.Download,
				ExpiresAt: time.Now().Add(4 * time.Hour),
			})
			return rdResp.Download, nil
		}
	}

	errBody, _ := io.ReadAll(resp.Body)
	return "", errors.New("RD unrestrict failed: " + string(errBody))
}

func (rd *RealDebridProvider) IsRateLimited() bool {
	rd.Mu.Lock()
	defer rd.Mu.Unlock()
	now := time.Now()
	for _, key := range rd.Keys {
		if key.LockedOutUntil.Before(now) {
			return false
		}
	}
	return len(rd.Keys) > 0 // true if we have keys and all are locked
}

// --- GLOBAL DEBRID MANAGER ---
var debridProviders []DebridProvider

func loadKeysFromDisk() []string {
	if data, err := os.ReadFile("/data/keys.json"); err == nil {
		var fileKeys []string
		if json.Unmarshal(data, &fileKeys) == nil {
			return fileKeys
		}
	}
	return nil
}

func saveKeysToDisk(keys []string) {
	if data, err := json.MarshalIndent(keys, "", "  "); err == nil {
		os.WriteFile("/data/keys.json", data, 0644)
	}
}

func initDebridPools() {
	rd := &RealDebridProvider{}

	rawKeys := os.Getenv("RD_API_KEY")
	for _, k := range strings.Split(rawKeys, ",") {
		if trimmed := strings.TrimSpace(k); trimmed != "" {
			rd.Keys = append(rd.Keys, &RdKey{Token: trimmed})
		}
	}

	diskKeys := loadKeysFromDisk()
	for _, k := range diskKeys {
		if trimmed := strings.TrimSpace(k); trimmed != "" {
			exists := false
			for _, existing := range rd.Keys {
				if existing.Token == trimmed {
					exists = true
					break
				}
			}
			if !exists {
				rd.Keys = append(rd.Keys, &RdKey{Token: trimmed})
			}
		}
	}

	if len(rd.Keys) > 0 {
		log.Printf("🔑 Registered Real-Debrid Provider (%d keys)", len(rd.Keys))
		debridProviders = append(debridProviders, rd)
	} else {
		log.Printf("⚠️ WARNING: No valid Debrid tokens found! Add some via Web UI.")
	}

	// Load AllDebrid
	adKey := os.Getenv("AD_API_KEY")
	if adKey != "" {
		ad := &AllDebridProvider{Token: strings.TrimSpace(adKey)}
		debridProviders = append(debridProviders, ad)
		log.Printf("🔑 Registered AllDebrid Provider")
	}

	// Load Premiumize
	pmKey := os.Getenv("PM_API_KEY")
	if pmKey != "" {
		pm := &PremiumizeProvider{Token: strings.TrimSpace(pmKey)}
		debridProviders = append(debridProviders, pm)
		log.Printf("🔑 Registered Premiumize Provider")
	}

	// Load Offcloud
	ocKey := os.Getenv("OC_API_KEY")
	if ocKey != "" {
		oc := &OffcloudProvider{Token: strings.TrimSpace(ocKey)}
		debridProviders = append(debridProviders, oc)
		log.Printf("🔑 Registered Offcloud Provider")
	}
}

// --- ALLDEBRID IMPLEMENTATION ---
type AllDebridProvider struct {
	Token          string
	LockedOutUntil time.Time
	Mu             sync.Mutex
}

func (ad *AllDebridProvider) Name() string    { return "AllDebrid" }
func (ad *AllDebridProvider) Hosts() []string { return []string{"alldebrid.com", "alldebrid.fr"} }

func (ad *AllDebridProvider) Unrestrict(link string) (string, error) {
	if ad.Token == "" {
		return "", errors.New("no AD keys available")
	}

	apiURL := "https://api.alldebrid.com/v4/link/unlock?agent=handoff&apikey=" + ad.Token + "&link=" + url.QueryEscape(link)

	req, _ := http.NewRequest("GET", apiURL, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		ad.Mu.Lock()
		ad.LockedOutUntil = time.Now().Add(5 * time.Minute)
		ad.Mu.Unlock()
		log.Printf("🔑 [Rate Limit] AllDebrid rate limited — locking out for 5 mins")
		return "", errors.New("AD rate limited")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.New("AD unrestrict: failed to read response body")
	}

	var adResp struct {
		Status string `json:"status"`
		Data   struct {
			Link string `json:"link"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &adResp); err == nil {
		if adResp.Status == "success" && adResp.Data.Link != "" {
			return adResp.Data.Link, nil
		}
		if adResp.Status == "error" {
			errMsg := adResp.Error.Message
			if errMsg == "" {
				errMsg = adResp.Error.Code
			}
			log.Printf("❌ [AD] API error: %s (HTTP %d)", errMsg, resp.StatusCode)
			return "", errors.New("AD unrestrict error: " + errMsg)
		}
	}
	log.Printf("❌ [AD] Unrestrict failed (HTTP %d): %s", resp.StatusCode, string(body))
	return "", errors.New("AD unrestrict failed (HTTP " + http.StatusText(resp.StatusCode) + "): " + string(body))
}

func (ad *AllDebridProvider) IsRateLimited() bool {
	ad.Mu.Lock()
	defer ad.Mu.Unlock()
	return time.Now().Before(ad.LockedOutUntil)
}

// --- PREMIUMIZE IMPLEMENTATION ---
type PremiumizeProvider struct {
	Token string
}

func (pm *PremiumizeProvider) Name() string    { return "Premiumize" }
func (pm *PremiumizeProvider) Hosts() []string { return []string{"premiumize.me"} }

func (pm *PremiumizeProvider) Unrestrict(link string) (string, error) {
	if pm.Token == "" {
		return "", errors.New("no PM keys available")
	}

	apiURL := "https://www.premiumize.me/api/transfer/directdl"
	payload := "src=" + url.QueryEscape(link)

	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+pm.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.New("PM unrestrict: failed to read response body")
	}

	var pmResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Content []struct {
			Link string `json:"link"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &pmResp); err == nil {
		if pmResp.Status == "success" && len(pmResp.Content) > 0 {
			return pmResp.Content[0].Link, nil
		}
		if pmResp.Status == "error" {
			log.Printf("❌ [PM] API error (HTTP %d): %s", resp.StatusCode, pmResp.Message)
			return "", errors.New("PM unrestrict error: " + pmResp.Message)
		}
	}
	log.Printf("❌ [PM] Unrestrict failed (HTTP %d): %s", resp.StatusCode, string(body))
	return "", errors.New("PM unrestrict failed (HTTP " + http.StatusText(resp.StatusCode) + "): " + string(body))
}

func (pm *PremiumizeProvider) IsRateLimited() bool { return false }

// --- OFFCLOUD IMPLEMENTATION ---
type OffcloudProvider struct {
	Token string
}

func (oc *OffcloudProvider) Name() string    { return "Offcloud" }
func (oc *OffcloudProvider) Hosts() []string { return []string{"offcloud.com"} }

func (oc *OffcloudProvider) Unrestrict(link string) (string, error) {
	if oc.Token == "" {
		return "", errors.New("no OC keys available")
	}

	apiURL := "https://offcloud.com/api/cloud/download"
	payload := "url=" + url.QueryEscape(link)

	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+oc.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.New("OC unrestrict: failed to read response body")
	}

	var ocResp struct {
		Url   string `json:"url"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &ocResp); err == nil {
		if ocResp.Url != "" {
			return ocResp.Url, nil
		}
		if ocResp.Error != "" {
			log.Printf("❌ [OC] API error (HTTP %d): %s", resp.StatusCode, ocResp.Error)
			return "", errors.New("OC unrestrict error: " + ocResp.Error)
		}
	}
	log.Printf("❌ [OC] Unrestrict failed (HTTP %d): %s", resp.StatusCode, string(body))
	return "", errors.New("OC unrestrict failed (HTTP " + http.StatusText(resp.StatusCode) + "): " + string(body))
}

func (oc *OffcloudProvider) IsRateLimited() bool { return false }

func getDebridProviderForHost(link string) DebridProvider {
	for _, p := range debridProviders {
		for _, host := range p.Hosts() {
			if strings.Contains(link, host) {
				return p
			}
		}
	}
	return nil
}

// Keeping legacy support for API functions that specifically fetch RD keys
func getRdApiKey() string {
	for _, p := range debridProviders {
		if rd, ok := p.(*RealDebridProvider); ok {
			return rd.getActiveToken()
		}
	}
	return ""
}

// rdAddMagnet adds a magnet link or infoHash to Real-Debrid's download queue.
// Returns an error if the submission fails, including the HTTP status code.
func rdAddMagnet(magnetOrHash string) error {
	token := getRdApiKey()
	if token == "" {
		return errors.New("no RD keys available")
	}

	// Normalise to a magnet URI if it looks like a bare infoHash
	magnet := magnetOrHash
	if !strings.HasPrefix(strings.ToLower(magnetOrHash), "magnet:") {
		magnet = "magnet:?xt=urn:btih:" + magnetOrHash
	}

	payload := "magnet=" + url.QueryEscape(magnet)
	req, err := http.NewRequest("POST",
		"https://api.real-debrid.com/rest/1.0/torrents/addMagnet",
		strings.NewReader(payload))
	if err != nil {
		log.Printf("[RD] ❌ Failed to build addMagnet request: %v", err)
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := rdDo(req)
	if err != nil {
		log.Printf("[RD] ❌ addMagnet request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 451 {
		// Infringing file — RD will never cache this, skip silently
		log.Printf("[RD] ⚠️ addMagnet HTTP 451 (infringing file) — skipping hash")
		return fmt.Errorf("http_%d", resp.StatusCode)
	}

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[RD] ❌ addMagnet returned HTTP %d: %s", resp.StatusCode, body)
		return fmt.Errorf("http_%d", resp.StatusCode)
	}

	var addResp struct {
		ID  string `json:"id"`
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&addResp); err != nil || addResp.ID == "" {
		log.Printf("[RD] ❌ addMagnet: could not parse response")
		return errors.New("addMagnet: could not parse response")
	}

	log.Printf("[RD] ✅ Torrent added to RD queue (id=%s). Selecting all files...", addResp.ID)

	// Select all files so RD starts downloading
	selectReq, err := http.NewRequest("POST",
		"https://api.real-debrid.com/rest/1.0/torrents/selectFiles/"+addResp.ID,
		strings.NewReader("files=all"))
	if err != nil {
		log.Printf("[RD] ⚠️ Failed to build selectFiles request: %v", err)
		return nil // torrent was added, just couldn't select files
	}
	selectReq.Header.Set("Authorization", "Bearer "+token)
	selectReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	selectResp, err := rdDo(selectReq)
	if err != nil {
		log.Printf("[RD] ⚠️ selectFiles request failed: %v", err)
		return nil
	}
	selectResp.Body.Close()
	log.Printf("[RD] 📥 Torrent files selected (id=%s, HTTP %d) — will be cached for next request", addResp.ID, selectResp.StatusCode)
	return nil
}
