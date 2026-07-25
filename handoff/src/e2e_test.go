package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestStremioE2EWorkflow(t *testing.T) {
	// 1. Initialize global dependencies
	initDB()
	initCache()
	aliasMappings.Clear()

	// 2. Setup Upstream Mock Stremio Addon
	upstreamMux := http.NewServeMux()
	upstreamMux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"id":"org.e2e.mock","version":"1.0.0","name":"E2E Mock","resources":["catalog","stream"],"types":["movie","series"]}`)); err != nil {
			t.Logf("Failed to write mock response: %v", err)
		}
	})
	upstreamMux.HandleFunc("/catalog/movie/top.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"metas":[{"id":"tt1234567","type":"movie","name":"Test Movie"}]}`)); err != nil {
			t.Logf("Failed to write mock response: %v", err)
		}
	})
	upstreamMux.HandleFunc("/stream/movie/tt1234567.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"streams":[{"url":"http://fake.provider.com/download.mp4","name":"1080p Stream"}]}`)); err != nil {
			t.Logf("Failed to write mock response: %v", err)
		}
	})

	upstreamServer := httptest.NewServer(upstreamMux)
	defer upstreamServer.Close()

	// 3. Create Alias Mapping for the Mock Addon
	mockAddonURL := upstreamServer.URL + "/manifest.json"
	aliasMappings.Store("e2e", Config{AddonURL: mockAddonURL})

	// 4. Create local Handoff Engine Router
	handoffHandler := http.HandlerFunc(routeHandler)

	// --- STEP 1: LOAD MANIFEST ---
	reqManifest, _ := http.NewRequest("GET", "/e2e/manifest.json", nil)
	rrManifest := httptest.NewRecorder()
	handoffHandler.ServeHTTP(rrManifest, reqManifest)

	if rrManifest.Code != http.StatusOK {
		t.Fatalf("Step 1 Failed: Expected 200 OK for manifest, got %d", rrManifest.Code)
	}
	if !strings.Contains(rrManifest.Body.String(), "org.e2e.mock.handoff") {
		t.Fatalf("Step 1 Failed: Manifest body missing upstream ID")
	}

	// --- STEP 2: LOAD CATALOG ---
	reqCatalog, _ := http.NewRequest("GET", "/e2e/catalog/movie/top.json", nil)
	rrCatalog := httptest.NewRecorder()
	handoffHandler.ServeHTTP(rrCatalog, reqCatalog)

	if rrCatalog.Code != http.StatusOK {
		t.Fatalf("Step 2 Failed: Expected 200 OK for catalog, got %d", rrCatalog.Code)
	}
	if !strings.Contains(rrCatalog.Body.String(), "tt1234567") {
		t.Fatalf("Step 2 Failed: Catalog body missing expected metadata")
	}

	// --- STEP 3: LOAD STREAMS (Picking a result) ---
	reqStream, _ := http.NewRequest("GET", "/e2e/stream/movie/tt1234567.json", nil)
	rrStream := httptest.NewRecorder()
	handoffHandler.ServeHTTP(rrStream, reqStream)

	if rrStream.Code != http.StatusOK {
		t.Fatalf("Step 3 Failed: Expected 200 OK for streams, got %d", rrStream.Code)
	}

	// Parse streams to find the wrapped URL
	var streamResp struct {
		Streams []map[string]interface{} `json:"streams"`
	}
	if err := json.Unmarshal(rrStream.Body.Bytes(), &streamResp); err != nil {
		t.Fatalf("Step 3 Failed: Invalid JSON response: %v", err)
	}
	if len(streamResp.Streams) == 0 {
		t.Fatalf("Step 3 Failed: Expected at least 1 stream, got 0")
	}

	wrappedURL, ok := streamResp.Streams[0]["url"].(string)
	if !ok || !strings.Contains(wrappedURL, "/e2e/play?link=") {
		t.Fatalf("Step 3 Failed: Stream URL was not wrapped correctly. Got: %v", wrappedURL)
	}

	// Ensure the embedded stream correctly URL encoded the upstream HTTP link
	expectedTarget := url.QueryEscape("http://fake.provider.com/download.mp4")
	if !strings.Contains(wrappedURL, expectedTarget) {
		t.Fatalf("Step 3 Failed: Stream URL does not contain expected target link. Got: %s", wrappedURL)
	}

	// --- STEP 4: LOAD STREAM (Clicking a result to play) ---
	// Wait a moment for async DB insertion of stream URLs
	time.Sleep(200 * time.Millisecond)

	// Extract the relative path from the wrapped URL
	parsedURL, _ := url.Parse(wrappedURL)
	reqPlay, _ := http.NewRequest("GET", parsedURL.RequestURI(), nil)
	rrPlay := httptest.NewRecorder()
	handoffHandler.ServeHTTP(rrPlay, reqPlay)

	// Since "fake.provider.com" isn't a known Debrid host, Handoff will follow redirects
	// (which fails because fake.provider.com doesn't exist), then it will ultimately issue a 302 Found
	// to redirect the client to the native finalURL.
	if rrPlay.Code != http.StatusFound {
		t.Fatalf("Step 4 Failed: Expected 302 Redirect to bypass non-debrid link, got %d. Body: %s", rrPlay.Code, rrPlay.Body.String())
	}

	loc := rrPlay.Header().Get("Location")
	if loc != "http://fake.provider.com/download.mp4" {
		t.Fatalf("Step 4 Failed: Expected redirection to original link, got %s", loc)
	}
}
