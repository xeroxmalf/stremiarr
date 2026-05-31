package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestInitRdPool(t *testing.T) {
	// Backup and restore env
	oldEnv := os.Getenv("RD_API_KEY")
	defer os.Setenv("RD_API_KEY", oldEnv)

	// Test single token
	os.Setenv("RD_API_KEY", "token1")
	rdApiKeys = nil
	initRdPool()
	if len(rdApiKeys) != 1 || rdApiKeys[0] != "token1" {
		t.Errorf("Expected 1 token 'token1', got %v", rdApiKeys)
	}

	// Test multiple tokens
	rdApiKeys = nil
	os.Setenv("RD_API_KEY", "token1, token2 ,token3")
	initRdPool()
	if len(rdApiKeys) != 3 {
		t.Errorf("Expected 3 tokens, got %d", len(rdApiKeys))
	}
	expected := []string{"token1", "token2", "token3"}
	for i, v := range expected {
		if rdApiKeys[i] != v {
			t.Errorf("Expected token %d to be %s, got %s", i, v, rdApiKeys[i])
		}
	}
}

func TestGetRdApiKey(t *testing.T) {
	rdApiKeys = []string{"t1", "t2"}
	rdTokenIdx = 0

	// atomic.AddUint64 starts at 1
	k1 := getRdApiKey() // idx 1 % 2 = 1 -> "t2"
	k2 := getRdApiKey() // idx 2 % 2 = 0 -> "t1"
	k3 := getRdApiKey() // idx 3 % 2 = 1 -> "t2"

	if k1 != "t2" { t.Errorf("Expected t2, got %s", k1) }
	if k2 != "t1" { t.Errorf("Expected t1, got %s", k2) }
	if k3 != "t2" { t.Errorf("Expected t2, got %s", k3) }
}

func TestHealthEndpoint(t *testing.T) {
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(routeHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := "OK"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestIsSuspiciousURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://real-debrid.com/abcd", false},
		{"http://example.com/test", true},
		{"https://google.com", false},
		{"ftp://myserver.com", true},
		{"https://test.stream/video", true},
	}

	for _, tt := range tests {
		if got := isSuspiciousURL(tt.url); got != tt.expected {
			t.Errorf("isSuspiciousURL(%q) = %v; want %v", tt.url, got, tt.expected)
		}
	}
}
