package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeAPIMappings_POST(t *testing.T) {
	aliasMappings.Clear() // Ensure clean state

	// Test 1: Valid POST
	payload := []byte(`{"addon_url": "https://test.addon/manifest.json", "alias": "myalias"}`)
	req, _ := http.NewRequest("POST", "/api/mappings", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()

	serveAPIMappings(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d", rr.Code)
	}

	// Verify it was stored
	val, ok := aliasMappings.Load("myalias")
	if !ok {
		t.Errorf("Expected mapping to be stored")
	}
	conf := val.(Config)
	if conf.AddonURL != "https://test.addon/manifest.json" {
		t.Errorf("Expected AddonURL to be mapped correctly")
	}

	// Test 2: Missing Addon URL
	payload2 := []byte(`{"alias": "no-url"}`)
	req2, _ := http.NewRequest("POST", "/api/mappings", bytes.NewBuffer(payload2))
	rr2 := httptest.NewRecorder()

	serveAPIMappings(rr2, req2)

	if rr2.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", rr2.Code)
	}
}
