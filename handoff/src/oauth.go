package main

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"os"
)

func generateState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Printf("⚠️ Failed to read random bytes for state: %v", err)
	}
	return base64.URLEncoding.EncodeToString(b)
}

func handleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	clientID := os.Getenv("OAUTH_CLIENT_ID")
	authURL := os.Getenv("OAUTH_AUTHORIZE_URL")
	redirectURI := os.Getenv("OAUTH_REDIRECT_URI")

	if clientID == "" || authURL == "" || redirectURI == "" {
		http.Error(w, "OAuth is not configured", http.StatusNotImplemented)
		return
	}

	state := generateState()
	// Set state in cookie for CSRF protection
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})

	redirect := authURL + "?client_id=" + clientID + "&redirect_uri=" + redirectURI + "&response_type=code&scope=openid profile email&state=" + state

	log.Printf("🔑 Redirecting user to OAuth Provider: %s", authURL)
	http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
}

func handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	// In a complete implementation, this would exchange the code for an access token
	// and set the X-Stremiarr-Role cookie/header based on claims.
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	log.Printf("🔑 OAuth Callback received. Authenticating user...")

	// Issue a mock successful admin token for demonstration purposes
	http.SetCookie(w, &http.Cookie{
		Name:     "stremiarr_session",
		Value:    "authenticated_token",
		Path:     "/",
		HttpOnly: true,
	})

	http.Redirect(w, r, "/ui", http.StatusTemporaryRedirect)
}
