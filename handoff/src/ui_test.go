package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeUI(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	// Mock authentication for testing
	// Removed since basic auth is delegated to Caddy

	serveWebUI(rr, req)

	// Since we mock ui.html as a variable or read it from disk,
	// serveUI writes the embedded uiHTML.
	// Wait, ui.html is embedded. Let's just check the status code and Content-Type.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, "text/html; charset=utf-8")
	}

	if len(rr.Body.Bytes()) == 0 {
		t.Errorf("handler returned empty body")
	}
}
