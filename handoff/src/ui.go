package main

import (
	_ "embed"
	"net/http"
)

//go:embed ui.html
var uiTemplate string

func serveWebUI(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(uiTemplate))
}
