package main

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestLocalCatalogCache(t *testing.T) {
	lc := &LocalCache{}

	key := "test-catalog-key"
	body := []byte("dummy data")
	headers := make(http.Header)
	headers.Set("X-Test-Header", "value")
	statusCode := 200
	ttl := 1 * time.Second

	// Set cache
	err := lc.SetCatalogCache(key, body, headers, statusCode, ttl)
	if err != nil {
		t.Fatalf("Failed to set catalog cache: %v", err)
	}

	// Get cache (should hit)
	cachedBody, cachedHeaders, cachedStatus, err := lc.GetCatalogCache(key)
	if err != nil {
		t.Fatalf("Expected cache hit, but got error: %v", err)
	}
	if !bytes.Equal(cachedBody, body) {
		t.Errorf("Expected body %s, got %s", body, cachedBody)
	}
	if cachedHeaders.Get("X-Test-Header") != "value" {
		t.Errorf("Expected header value 'value', got %s", cachedHeaders.Get("X-Test-Header"))
	}
	if cachedStatus != statusCode {
		t.Errorf("Expected status %d, got %d", statusCode, cachedStatus)
	}

	// Wait for expiration
	time.Sleep(1500 * time.Millisecond)

	// Get cache (should miss)
	_, _, _, err = lc.GetCatalogCache(key)
	if err != redis.Nil {
		t.Fatalf("Expected redis.Nil error due to expiration, but got: %v", err)
	}
}
