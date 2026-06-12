package main

import (
	"net/http"
	"os"
	"sync"
	"time"
)

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
		{Name: "Comet", URL: "https://comet.feels.legal/manifest.json", Enabled: true},
		{Name: "TorrentIO", URL: "https://torrentio.strem.fun/manifest.json", Enabled: true},
		{Name: "MediaFusion", URL: "https://mediafusion.elfhosted.com/manifest.json", Enabled: true},
		{Name: "StremThru", URL: "https://stremthru.13377001.xyz/manifest.json", Enabled: true},
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
		return buf.([]byte)
	}
	return make([]byte, 128*1024)
}

func (p *bytesPool) Put(buf []byte) {
	p.pool.Put(buf)
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
