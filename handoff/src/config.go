package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

var DataDir = "/data"

func init() {
	if d := os.Getenv("DATA_DIR"); d != "" {
		DataDir = d
	}
	if err := os.MkdirAll(DataDir, 0755); err != nil {
		// Fallback to temp dir for CI/test environments where /data may not be writable
		tmp, err := os.MkdirTemp("", "handoff-*")
		if err != nil {
			panic("Failed to initialize DataDir (" + DataDir + ") and cannot create temp dir: " + err.Error())
		}
		DataDir = tmp
	}
}

type Config struct {
	AddonURL       string   `json:"addon_url"`
	TorrentioURL   string   `json:"torrentio_url"`
	MediafusionURL string   `json:"mediafusion_url"`
	StremthruURL   string   `json:"stremthru_url"`
	Plugins        []string `json:"plugins"` // Webhook URLs for modifying streams
}

type AddonSource struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

var (
	sourcesMu sync.RWMutex

	// Local Comet URL can be overridden via COMET_LOCAL_URL env var
	localCometURL = func() string {
		if u := os.Getenv("COMET_LOCAL_URL"); u != "" {
			return u
		}
		return "http://192.168.250.125:8000"
	}()

	addonSources = []AddonSource{
		{Name: "Comet (Local - DMM Search)", URL: localCometURL + "/eyJtYXhSZXN1bHRzUGVyUmVzb2x1dGlvbiI6MCwibWF4U2l6ZSI6MCwiY2FjaGVkT25seSI6dHJ1ZSwic29ydENhY2hlZFVuY2FjaGVkVG9nZXRoZXIiOmZhbHNlLCJyZW1vdmVUcmFzaCI6dHJ1ZSwicmVzdWx0Rm9ybWF0IjpbImFsbCJdLCJkZWJyaWRTZXJ2aWNlcyI6W3sic2VydmljZSI6InJlYWxkZWJyaWQiLCJhcGlLZXkiOiJBUlVGSFo0SjNZM1BPNEtXSDUzTVhNRTM3S09CQVhMS1o1N0NOTkhBV0k0N05MNTM2UTNRIn1dLCJlbmFibGVUb3JyZW50IjpmYWxzZSwiZGVkdXBsaWNhdGVTdHJlYW1zIjpmYWxzZSwic2NyYXBlRGVicmlkQWNjb3VudFRvcnJlbnRzIjp0cnVlLCJkZWJyaWRTdHJlYW1Qcm94eVBhc3N3b3JkIjoiIiwibGFuZ3VhZ2VzIjp7InJlcXVpcmVkIjpbXSwiYWxsb3dlZCI6W10sImV4Y2x1ZGUiOltdLCJwcmVmZXJyZWQiOltdfSwicmVzb2x1dGlvbnMiOnt9LCJvcHRpb25zIjp7InJlbW92ZV9yYW5rc191bmRlciI6LTEwMDAwMDAwMDAwLCJhbGxvd19lbmdsaXNoX2luX2xhbmd1YWdlcyI6ZmFsc2UsInJlbW92ZV91bmtub3duX2xhbmd1YWdlcyI6ZmFsc2V9fQ==/manifest.json", Enabled: true},
		{Name: "Comet (Public)", URL: "https://comet.feels.legal/eyJtYXhSZXN1bHRzUGVyUmVzb2x1dGlvbiI6MCwibWF4U2l6ZSI6MCwiY2FjaGVkT25seSI6dHJ1ZSwic29ydENhY2hlZFVuY2FjaGVkVG9nZXRoZXIiOmZhbHNlLCJyZW1vdmVUcmFzaCI6dHJ1ZSwicmVzdWx0Rm9ybWF0IjpbImFsbCJdLCJkZWJyaWRTZXJ2aWNlcyI6W3sic2VydmljZSI6InJlYWxkZWJyaWQiLCJhcGlLZXkiOiJBUlVGSFo0SjNZM1BPNEtXSDUzTVhNRTM3S09CQVhMS1o1N0NOTkhBV0k0N05MNTM2UTNRIn1dLCJlbmFibGVUb3JyZW50IjpmYWxzZSwiZGVkdXBsaWNhdGVTdHJlYW1zIjpmYWxzZSwic2NyYXBlRGVicmlkQWNjb3VudFRvcnJlbnRzIjp0cnVlLCJkZWJyaWRTdHJlYW1Qcm94eVBhc3N3b3JkIjoiIiwibGFuZ3VhZ2VzIjp7InJlcXVpcmVkIjpbXSwiYWxsb3dlZCI6W10sImV4Y2x1ZGUiOltdLCJwcmVmZXJyZWQiOltdfSwicmVzb2x1dGlvbnMiOnt9LCJvcHRpb25zIjp7InJlbW92ZV9yYW5rc191bmRlciI6LTEwMDAwMDAwMDAwLCJhbGxvd19lbmdsaXNoX2luX2xhbmd1YWdlcyI6ZmFsc2UsInJlbW92ZV91bmtub3duX2xhbmd1YWdlcyI6ZmFsc2V9fQ==/manifest.json", Enabled: true},
		{Name: "TorrentIO", URL: "https://torrentio.strem.fun/manifest.json", Enabled: true},
		{Name: "MediaFusion", URL: "https://mediafusion.elfhosted.com/manifest.json", Enabled: true},
		{Name: "StremThru", URL: "https://stremthru.13377001.xyz/manifest.json", Enabled: true},
		{Name: "DMM Cast", URL: "https://debridmediamanager.com/api/stremio/a514a2e420c02f0a53565b3578502b9f36281a258aacc4cf3be9d1670eb9864b/manifest.json", Enabled: true},
	}
)

var RcloneUrl = os.Getenv("RCLONE_URL")
var RcloneAuth = os.Getenv("RCLONE_AUTH")
var RcloneRcUrl = os.Getenv("RCLONE_RC_URL")
var AdminPassword = os.Getenv("ADMIN_PASSWORD")

var startTime = time.Now()

// --- ZERO-STUTTER BUFFER POOL ---
type bytesPool struct {
	pool sync.Pool
}

type byteSlice struct {
	Buf []byte
}

func (p *bytesPool) Get() []byte {
	if bs := p.pool.Get(); bs != nil {
		bs := bs.(*byteSlice)
		return bs.Buf[:128*1024]
	}
	return make([]byte, 128*1024)
}

func (p *bytesPool) Put(buf []byte) {
	if cap(buf) == 128*1024 {
		p.pool.Put(&byteSlice{Buf: buf[:128*1024]})
	}
}

var proxyPool = &bytesPool{}

// customTransport: tuned for stable video streaming (no HTTP/2 to avoid head-of-line blocking)
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
	WriteBufferSize:       64 * 1024,
}

// defaultTransport: general-purpose API calls with HTTP/2 support
var defaultTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   50,
	MaxConnsPerHost:       50,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	DisableCompression:    false,
	ForceAttemptHTTP2:     true,
}

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		return nil
	},
	Transport: defaultTransport,
}
