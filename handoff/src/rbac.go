package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// sessionEntry holds session metadata for rotation/expiration
type sessionEntry struct {
	token     string
	createdAt time.Time
	lastUsed  time.Time
}

var (
	// Session store (in-memory)
	sessionsMu     sync.RWMutex
	activeSessions = make(map[string]*sessionEntry)
	sessionTTL     = 7 * 24 * time.Hour // 7 days
)

// RequireRole is a middleware that enforces Role-Based Access Control (RBAC) on API endpoints.
// Supports two auth methods:
// 1. X-Stremiarr-Role header (for service-to-service)
// 2. ADMIN_PASSWORD environment variable (for web UI)
func RequireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Method 1: Header-based RBAC
		if os.Getenv("RBAC_ENABLED") == "true" {
			token := r.Header.Get("X-Stremiarr-Role")
			if token != role && token != "admin" {
				log.Printf("🔐 RBAC Blocked: user with role '%s' attempted to access endpoint requiring '%s'", token, role)
				http.Error(w, "Forbidden - Insufficient Permissions", http.StatusForbidden)
				return
			}
			next(w, r)
			return
		}

		// Method 2: Admin password cookie/session
		if AdminPassword != "" {
			// Check for session cookie
			cookie, err := r.Cookie("handoff_session")
			if err == nil && cookie != nil && validateSession(cookie.Value) {
				next(w, r)
				return
			}

			// For API endpoints, return 401
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("WWW-Authenticate", "Basic realm=\"Handoff\"")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// For UI, redirect to login
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// No auth configured, allow access
		next(w, r)
	}
}

// generateToken creates a cryptographically secure random token
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// validateSession checks if a session token is valid (not expired)
func validateSession(token string) bool {
	if token == "" {
		return false
	}
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	entry, ok := activeSessions[token]
	if !ok {
		return false
	}
	if time.Since(entry.createdAt) > sessionTTL {
		delete(activeSessions, token)
		return false
	}
	entry.lastUsed = time.Now()
	return true
}

// createSession creates a new session token
func createSession(password string) string {
	if password != AdminPassword {
		return ""
	}
	token := generateToken()
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	activeSessions[token] = &sessionEntry{
		token:     token,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}
	return token
}

// cleanupExpiredSessions removes expired sessions from the store
func cleanupExpiredSessions() {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	now := time.Now()
	for token, entry := range activeSessions {
		if now.Sub(entry.createdAt) > sessionTTL {
			delete(activeSessions, token)
		}
	}
}

// revokeSession invalidates a session token
func revokeSession(token string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	delete(activeSessions, token)
}

// initSessionCleanup starts a background goroutine to periodically clean up expired sessions
func initSessionCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanupExpiredSessions()
		}
	}()
}
