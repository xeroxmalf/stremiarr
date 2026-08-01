package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	initDB()
	initDBMaintenance()
	initCache()
	initCatalogCache()
	initSubtitleCache()
	initSessionCleanup()
	initDebridPools()
	initValidationPool()
	initAddonSources()
	StartAnomalyWorker()
	initRateLimiter()
	initWebSocketHub()
	initMetrics()

	if RcloneUrl == "" {
		log.Fatal("❌ ERROR: RCLONE_URL is missing!")
	}

	loadMappings()
	loadPrefetchHistory()

	// Load saved Stremio auth key if available
	if key := loadStremioAuth(); key != "" {
		stremioAuthMu.Lock()
		stremioAuthKey = key
		stremioAuthMu.Unlock()
		log.Printf("🎬 Loaded saved Stremio auth key")
	}

	log.Printf("🔐 Admin auth: %s", func() string {
		if AdminPassword != "" {
			return "ENABLED"
		}
		return "disabled"
	}())

	http.HandleFunc("/", gzipMiddleware(rateLimitMiddleware(observeRequestDuration(routeHandler))))

	port := os.Getenv("PORT")
	if port == "" {
		port = "9944"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown handler
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Printf("🛑 Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("⚠️ Server forced to shutdown: %v", err)
		}
		log.Printf("✅ Server shutdown complete")
	}()

	log.Printf("🚀 Handoff Engine Initialized!")
	log.Printf("📡 Listening on port %s", port)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}
