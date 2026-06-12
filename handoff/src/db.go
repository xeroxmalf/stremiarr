package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type DBWrapper struct {
	conn *sql.DB
	isPg bool
}

func (w *DBWrapper) bind(query string) string {
	if !w.isPg {
		return query
	}
	var res string
	var count int
	for _, c := range query {
		if c == '?' {
			count++
			res += fmt.Sprintf("$%d", count)
		} else {
			res += string(c)
		}
	}
	return res
}

func (w *DBWrapper) Exec(query string, args ...interface{}) (sql.Result, error) {
	return w.conn.Exec(w.bind(query), args...)
}

func (w *DBWrapper) QueryRow(query string, args ...interface{}) *sql.Row {
	return w.conn.QueryRow(w.bind(query), args...)
}

func (w *DBWrapper) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return w.conn.Query(w.bind(query), args...)
}

func (w *DBWrapper) Close() error {
	return w.conn.Close()
}

var db *DBWrapper

func initDB() {
	dbType := os.Getenv("DATABASE_TYPE")
	dbUrl := os.Getenv("DATABASE_URL")

	var conn *sql.DB
	var err error
	var isPg bool

	if dbType == "postgresql" {
		conn, err = sql.Open("postgres", dbUrl)
		if err != nil {
			log.Fatalf("❌ Failed to open Postgres DB: %v", err)
		}
		isPg = true
		log.Printf("💾 Postgres database initialized")
	} else {
		os.MkdirAll("/data", 0755)
		conn, err = sql.Open("sqlite", "/data/streams.db?_busy_timeout=5000&_journal_mode=WAL&_sync=NORMAL&_cache_size=-20000")
		if err != nil {
			log.Fatalf("❌ Failed to open SQLite DB: %v", err)
		}
		conn.Exec("PRAGMA temp_store = MEMORY;")
		isPg = false
		log.Printf("💾 SQLite database initialized at /data/streams.db")
	}

	db = &DBWrapper{conn: conn, isPg: isPg}
	runMigrations()
}

func runMigrations() {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS handoff_migrations (
			version INTEGER PRIMARY KEY
		);
	`)
	if err != nil {
		log.Fatalf("❌ Failed to init handoff_migrations: %v", err)
	}

	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM handoff_migrations").Scan(&currentVersion)
	if err != nil {
		log.Fatalf("❌ Failed to query current schema version: %v", err)
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS stream_cache (
			request_id TEXT PRIMARY KEY,
			streams_json TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE TABLE IF NOT EXISTS bandwidth_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			bytes INTEGER NOT NULL,
			source TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS stream_urls (
            url TEXT PRIMARY KEY,
            is_valid BOOLEAN DEFAULT TRUE,
            fail_count INTEGER DEFAULT 0,
            last_validated TIMESTAMP
        );`,
		`ALTER TABLE stream_urls ADD COLUMN success_count INTEGER DEFAULT 0;`,
		`CREATE INDEX IF NOT EXISTS idx_stream_cache_updated_at ON stream_cache(updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_stream_urls_last_validated ON stream_urls(last_validated);`,
	}

	for i, migration := range migrations {
		version := i + 1
		if version > currentVersion {
			_, err := db.Exec(migration)
			if err != nil {
				log.Printf("⚠️ Migration %d warning (might already exist): %v", version, err)
			}
			db.Exec("INSERT INTO handoff_migrations (version) VALUES (?)", version)
			log.Printf("🔄 Applied DB migration v%d", version)
		}
	}
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
