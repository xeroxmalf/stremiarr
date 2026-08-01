package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var (
	// Default rate limit: 15 req/s with 75 burst (relaxed from 10/50 to reduce false positives)
	defaultLimiterRate  = rate.Limit(15)
	defaultLimiterBurst = 75

	// Strict rate limit for API endpoints (lower to prevent abuse)
	apiLimiterRate  = rate.Limit(5)
	apiLimiterBurst = 20

	clients   = make(map[string]*clientLimiter)
	clientsMu sync.Mutex
)

type clientLimiter struct {
	defaultLimiter *rate.Limiter
	apiLimiter     *rate.Limiter
	lastSeen       time.Time
}

func initRateLimiter() {
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			clientsMu.Lock()
			for ip, client := range clients {
				if time.Since(client.lastSeen) > 10*time.Minute {
					delete(clients, ip)
				}
			}
			clientsMu.Unlock()
		}
	}()
	log.Printf("🛡️ Rate Limiter initialized (15 req/s default, 5 req/s API, 75 burst)")
}

func getLimiter(ip string) *rate.Limiter {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	client, exists := clients[ip]
	if !exists {
		client = &clientLimiter{
			defaultLimiter: rate.NewLimiter(defaultLimiterRate, defaultLimiterBurst),
			apiLimiter:     rate.NewLimiter(apiLimiterRate, apiLimiterBurst),
			lastSeen:       time.Now(),
		}
		clients[ip] = client
		return client.defaultLimiter
	}

	client.lastSeen = time.Now()
	return client.defaultLimiter
}

func getAPILimiter(ip string) *rate.Limiter {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	client, exists := clients[ip]
	if !exists {
		client = &clientLimiter{
			defaultLimiter: rate.NewLimiter(defaultLimiterRate, defaultLimiterBurst),
			apiLimiter:     rate.NewLimiter(apiLimiterRate, apiLimiterBurst),
			lastSeen:       time.Now(),
		}
		clients[ip] = client
		return client.apiLimiter
	}

	client.lastSeen = time.Now()
	return client.apiLimiter
}

// trustedProxies defines CIDRs allowed to set X-Forwarded-For/X-Real-IP.
// In production, only your reverse proxy should be in this list.
var trustedProxies = func() []*net.IPNet {
	trusted := os.Getenv("TRUSTED_PROXY_CIDRS")
	if trusted == "" {
		trusted = "127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	}
	var nets []*net.IPNet
	for _, cidr := range strings.Split(trusted, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, net, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, net)
		}
	}
	return nets
}()

func isTrustedProxy(ip net.IP) bool {
	for _, cidr := range trustedProxies {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// Extract the true IP address from a request, accounting for reverse proxies.
// Only trusts X-Forwarded-For/X-Real-IP from known proxy IPs.
func getIP(r *http.Request) string {
	// Get the immediate client IP
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	clientIP := net.ParseIP(ip)

	// Only trust forwarded headers if the request comes from a trusted proxy
	if isTrustedProxy(clientIP) {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ips := strings.Split(forwarded, ",")
			return strings.TrimSpace(ips[0])
		}
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			return strings.TrimSpace(realIP)
		}
	}

	return ip
}

// rateLimitMiddleware wraps an http.HandlerFunc to provide per-IP rate limiting
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Whitelist health checks and metrics
		path := r.URL.Path
		if path == "/health" || path == "/metrics" {
			next(w, r)
			return
		}

		ip := getIP(r)

		// Use stricter limits for API endpoints
		var limiter *rate.Limiter
		if strings.HasPrefix(path, "/api/") {
			limiter = getAPILimiter(ip)
		} else {
			limiter = getLimiter(ip)
		}

		if !limiter.Allow() {
			http.Error(w, "429 Too Many Requests - Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}
