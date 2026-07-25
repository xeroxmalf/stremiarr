package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// gzipResponseWriter wraps an http.ResponseWriter to compress output.
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// gzipMiddleware compresses HTTP responses if the client supports it.
func gzipMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only compress if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r)
			return
		}

		// Skip compression for video streams and WebSockets
		if strings.Contains(r.URL.Path, "/transcode") || strings.Contains(r.URL.Path, "/play/") || strings.Contains(r.URL.Path, "/ws") {
			next(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		// Vary must be set to tell caches that response depends on Accept-Encoding
		w.Header().Add("Vary", "Accept-Encoding")

		gz := gzip.NewWriter(w)
		defer gz.Close()

		gzw := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		next(gzw, r)
	}
}
