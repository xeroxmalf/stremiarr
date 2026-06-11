package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeAPIKeys_GET(t *testing.T) {
	// Setup test provider
	debridProviders = nil
	rd := &RealDebridProvider{}
	rd.Keys = append(rd.Keys, &RdKey{Token: "test_token_123456789"})
	debridProviders = append(debridProviders, rd)

	req, err := http.NewRequest("GET", "/api/keys", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	serveAPIKeys(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	keys, ok := resp["keys"].([]interface{})
	if !ok || len(keys) != 1 {
		t.Fatalf("Expected 1 key in response, got %v", keys)
	}

	keyObj := keys[0].(map[string]interface{})
	tokenDisplay := keyObj["token"].(string)

	// Check if token is masked properly (first 4 and last 4)
	if !strings.HasPrefix(tokenDisplay, "test") || !strings.HasSuffix(tokenDisplay, "6789") || !strings.Contains(tokenDisplay, "...") {
		t.Errorf("Expected token to be masked, got %s", tokenDisplay)
	}
}

func TestServeAPIKeys_POST(t *testing.T) {
	debridProviders = nil
	rd := &RealDebridProvider{}
	debridProviders = append(debridProviders, rd)

	payload := []byte(`{"token": "new_secret_token"}`)
	req, err := http.NewRequest("POST", "/api/keys", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	serveAPIKeys(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	if len(rd.Keys) != 1 || rd.Keys[0].Token != "new_secret_token" {
		t.Errorf("Failed to add key to RealDebridProvider: %v", rd.Keys)
	}
}

func TestServeAPIKeys_DELETE(t *testing.T) {
	debridProviders = nil
	rd := &RealDebridProvider{}
	rd.Keys = append(rd.Keys, &RdKey{Token: "test_token_123456789"})
	debridProviders = append(debridProviders, rd)

	req, err := http.NewRequest("DELETE", "/api/keys?token=test_token_123456789", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	serveAPIKeys(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	if len(rd.Keys) != 0 {
		t.Errorf("Failed to delete key, remaining keys: %v", rd.Keys)
	}
}
