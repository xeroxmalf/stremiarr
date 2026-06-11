package main

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var (
	// Rate limit: 10 requests per second, with a burst of 50
	limiterRate  = rate.Limit(10)
	limiterBurst = 50

	clients   = make(map[string]*clientLimiter)
	clientsMu sync.Mutex
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func initRateLimiter() {
	go func() {
		// Clean up old IP limiters every 5 minutes
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
	log.Printf("🛡️ Rate Limiter initialized (10 req/s, 50 burst)")
}

func getLimiter(ip string) *rate.Limiter {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	client, exists := clients[ip]
	if !exists {
		limiter := rate.NewLimiter(limiterRate, limiterBurst)
		clients[ip] = &clientLimiter{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	client.lastSeen = time.Now()
	return client.limiter
}

// Extract the true IP address from a request, accounting for reverse proxies
func getIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// rateLimitMiddleware wraps an http.HandlerFunc to provide per-IP rate limiting
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getIP(r)
		limiter := getLimiter(ip)

		if !limiter.Allow() {
			http.Error(w, "429 Too Many Requests - Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}
