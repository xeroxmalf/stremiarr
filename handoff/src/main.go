package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime)

	initDB()
	initDBMaintenance()
	initCache()
	initDebridPools()
	initValidationPool()
	initCatalogCache()
	StartAnomalyWorker()
	initRateLimiter()
	initWebSocketHub()

	if RcloneUrl == "" {
		log.Fatal("❌ ERROR: RCLONE_URL is missing!")
	}

	loadMappings()
	initAddonSources()

	// Load saved Stremio auth key if available
	if key := loadStremioAuth(); key != "" {
		stremioAuthMu.Lock()
		stremioAuthKey = key
		stremioAuthMu.Unlock()
		log.Printf("🎬 Loaded saved Stremio auth key")
	}

	http.HandleFunc("/", gzipMiddleware(rateLimitMiddleware(routeHandler)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 DebridHandoff Engine Initialized!")
	log.Printf("📡 Listening for Stremio traffic on port %s", port)

	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
