package main

import (
	_ "embed"
	"log"
	"net/http"
)

//go:embed ui.html
var uiTemplate string

func serveWebUI(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(uiTemplate)); err != nil {
		log.Printf("⚠️ Failed to write UI template: %v", err)
	}
}
