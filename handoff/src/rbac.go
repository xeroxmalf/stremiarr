package main

import (
	"log"
	"net/http"
	"os"
)

// RequireRole is a middleware that enforces Role-Based Access Control (RBAC) on API endpoints.
func RequireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("RBAC_ENABLED") == "true" {
			token := r.Header.Get("X-Stremiarr-Role")
			if token != role && token != "admin" {
				log.Printf("🔐 RBAC Blocked: user with role '%s' attempted to access endpoint requiring '%s'", token, role)
				http.Error(w, "Forbidden - Insufficient Permissions", http.StatusForbidden)
				return
			}
			log.Printf("🔐 RBAC Granted: access to %s for role %s", r.URL.Path, role)
		}
		next(w, r)
	}
}
