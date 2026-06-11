package main

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestDBInitAndMaintenance(t *testing.T) {
	// Use an in-memory database for testing
	var err error
	db, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory db: %v", err)
	}
	defer db.Close()

	// 1. Setup tables
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS stream_cache (
            request_id TEXT PRIMARY KEY,
            streams_json TEXT,
            updated_at DATETIME
        );
        CREATE TABLE IF NOT EXISTS stream_urls (
            url TEXT PRIMARY KEY,
            is_valid BOOLEAN DEFAULT 1,
            fail_count INTEGER DEFAULT 0,
            success_count INTEGER DEFAULT 0,
            last_validated DATETIME
        );
    `)
	if err != nil {
		t.Fatalf("Failed to init tables: %v", err)
	}

	// Verify tables were created
	var count int
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('stream_cache', 'stream_urls')").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query tables: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 tables, got %d", count)
	}

	// 2. Test recordStrike
	testURL := "https://example.com/stream"

	// Insert dummy URL
	_, err = db.Exec("INSERT INTO stream_urls (url, fail_count, is_valid, last_validated) VALUES (?, 0, 1, ?)", testURL, time.Now())
	if err != nil {
		t.Fatalf("Failed to insert dummy URL: %v", err)
	}

	// Record strike
	recordStrike(testURL)

	// Verify strike was recorded
	var failCount int
	var isValid bool
	err = db.QueryRow("SELECT fail_count, is_valid FROM stream_urls WHERE url = ?", testURL).Scan(&failCount, &isValid)
	if err != nil {
		t.Fatalf("Failed to query strike: %v", err)
	}

	if failCount != 1 {
		t.Errorf("Expected fail_count 1, got %d", failCount)
	}
	if !isValid {
		t.Errorf("Expected is_valid to remain true after 1 strike")
	}

	// Strike 2 more times to trigger invalidation
	recordStrike(testURL)
	recordStrike(testURL)

	err = db.QueryRow("SELECT fail_count, is_valid FROM stream_urls WHERE url = ?", testURL).Scan(&failCount, &isValid)
	if err != nil {
		t.Fatalf("Failed to query strike: %v", err)
	}

	if failCount != 3 {
		t.Errorf("Expected fail_count 3, got %d", failCount)
	}
}
