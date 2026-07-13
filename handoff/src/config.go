package main

import (
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
		panic("Failed to initialize DataDir: " + err.Error())
	}
}

type Config struct {
	AddonURL       string `json:"addon_url"`
	TorrentioURL   string `json:"torrentio_url"`
	MediafusionURL string `json:"mediafusion_url"`
	StremthruURL   string   `json:"stremthru_url"`
	Plugins        []string `json:"plugins"` // Webhook URLs for modifying streams
}

type AddonSource struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

var (
	addonSources = []AddonSource{
		{Name: "Comet (Local - DMM Search)", URL: "http://comet:8000/manifest.json", Enabled: true},
		{Name: "Comet (Public)", URL: "https://comet.feels.legal/manifest.json", Enabled: true},
		{Name: "TorrentIO", URL: "https://torrentio.strem.fun/manifest.json", Enabled: true},
		{Name: "MediaFusion", URL: "https://mediafusion.elfhosted.com/manifest.json", Enabled: true},
		{Name: "StremThru", URL: "https://stremthru.13377001.xyz/manifest.json", Enabled: true},
		{Name: "DMM Cast", URL: "https://debridmediamanager.com/api/stremio/a514a2e420c02f0a53565b3578502b9f36281a258aacc4cf3be9d1670eb9864b/manifest.json", Enabled: true},
	}
	sourcesMu sync.Mutex
)

var RcloneUrl = os.Getenv("RCLONE_URL")
var RcloneAuth = os.Getenv("RCLONE_AUTH")
var RcloneRcUrl = os.Getenv("RCLONE_RC_URL")

var startTime = time.Now()

// --- ZERO-STUTTER BUFFER POOL ---
type bytesPool struct {
	pool sync.Pool
}

func (p *bytesPool) Get() []byte {
	if buf := p.pool.Get(); buf != nil {
		return *(buf.(*[]byte))
	}
	return make([]byte, 128*1024)
}

func (p *bytesPool) Put(buf []byte) {
	p.pool.Put(&buf)
}

var proxyPool = &bytesPool{}

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
