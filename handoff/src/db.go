package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// --- SQLITE DB CORE ---
var db *sql.DB

func initDB() {
	os.MkdirAll("/data", 0755)
	var err error
	// Optimized SQLite settings for higher throughput and concurrency
	db, err = sql.Open("sqlite", "/data/streams.db?_busy_timeout=5000&_journal_mode=WAL&_sync=NORMAL&_cache_size=-20000")
	if err != nil {
		log.Fatalf("❌ Failed to open SQLite DB: %v", err)
	}

	// Performance tuning: reduce IO by using memory for temp store
	_, err = db.Exec("PRAGMA temp_store = MEMORY;")
	if err != nil {
		log.Printf("⚠️ Warning: Failed to set temp_store PRAGMA: %v", err)
	}

	runMigrations()

	log.Printf("💾 SQLite database initialized at /data/streams.db")
}

func runMigrations() {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY
		);
	`)
	if err != nil {
		log.Fatalf("❌ Failed to init schema_migrations: %v", err)
	}

	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		log.Fatalf("❌ Failed to query current schema version: %v", err)
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS stream_cache (
            request_id TEXT PRIMARY KEY,
            streams_json TEXT,
            updated_at DATETIME
        );`,
		`CREATE TABLE IF NOT EXISTS stream_urls (
            url TEXT PRIMARY KEY,
            is_valid BOOLEAN DEFAULT 1,
            fail_count INTEGER DEFAULT 0,
            last_validated DATETIME
        );`,
		`ALTER TABLE stream_urls ADD COLUMN success_count INTEGER DEFAULT 0;`,
	}

	for i, migration := range migrations {
		version := i + 1
		if version > currentVersion {
			_, err := db.Exec(migration)
			if err != nil {
				log.Printf("⚠️ Migration %d warning (might already exist): %v", version, err)
			}
			db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version)
			log.Printf("🔄 Applied DB migration v%d", version)
		}
	}

	log.Printf("💾 SQLite database initialized at /data/streams.db")
}

func initDBMaintenance() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			res1, err1 := db.Exec("DELETE FROM stream_cache WHERE updated_at < ?", time.Now().Add(-7*24*time.Hour))
			res2, err2 := db.Exec("DELETE FROM stream_urls WHERE last_validated < ?", time.Now().Add(-30*24*time.Hour))

			var ra1, ra2 int64
			if err1 == nil {
				ra1, _ = res1.RowsAffected()
			}
			if err2 == nil {
				ra2, _ = res2.RowsAffected()
			}

			if ra1 > 0 || ra2 > 0 {
				log.Printf("🧹 [DB Maintenance] Pruned %d old stream_cache records and %d old stream_urls records", ra1, ra2)
			}
		}
	}()
}
