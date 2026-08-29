package main

import (
	"log"
	"sync"
	"time"
)

// bandwidthAggregator batches bandwidth records to avoid DB thrashing
var bwAgg = struct {
	mu        sync.Mutex
	records   map[string]int64 // source -> bytes
	interval  time.Duration
	lastFlush time.Time
}{
	records:  make(map[string]int64),
	interval: 1 * time.Minute,
}

func init() {
	go bwFlushLoop()
}

func bwFlushLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		bwFlush()
	}
}

func bwFlush() {
	bwAgg.mu.Lock()
	if len(bwAgg.records) == 0 {
		bwAgg.mu.Unlock()
		return
	}
	batch := make(map[string]int64, len(bwAgg.records))
	for k, v := range bwAgg.records {
		batch[k] = v
	}
	bwAgg.records = make(map[string]int64)
	bwAgg.mu.Unlock()

	for src, b := range batch {
		TrackBandwidthDB(int(b), src)
		TrackMetrics(b)
	}
}

// TrackBandwidthDB inserts a single bandwidth record into the database
func TrackBandwidthDB(bytes int, source string) {
	if db == nil {
		return
	}
	_, err := db.Exec("INSERT INTO bandwidth_log (bytes, source) VALUES (?, ?)", bytes, source)
	if err != nil {
		log.Printf("⚠️ Failed to log bandwidth: %v", err)
	}
}

// TrackBandwidth queues bytes for aggregated write to the database
func TrackBandwidth(bytes int, source string) {
	if bytes <= 0 {
		return
	}
	bwAgg.mu.Lock()
	bwAgg.records[source] += int64(bytes)
	bwAgg.mu.Unlock()
}

var bwStatsCache = struct {
	mu        sync.RWMutex
	stats     map[string]int64
	lastFetch time.Time
}{
	stats: make(map[string]int64),
}

// GetBandwidthStats returns the total bandwidth consumed grouped by source.
func GetBandwidthStats() map[string]int64 {
	bwStatsCache.mu.RLock()
	if !bwStatsCache.lastFetch.IsZero() && time.Since(bwStatsCache.lastFetch) < 30*time.Second {
		statsCopy := make(map[string]int64, len(bwStatsCache.stats))
		for k, v := range bwStatsCache.stats {
			statsCopy[k] = v
		}
		bwStatsCache.mu.RUnlock()
		return statsCopy
	}
	bwStatsCache.mu.RUnlock()

	stats := make(map[string]int64)
	if db == nil {
		return stats
	}

	rows, err := db.Query("SELECT source, SUM(bytes) FROM bandwidth_log GROUP BY source")
	if err != nil {
		log.Printf("⚠️ Failed to query bandwidth stats: %v", err)
		return stats
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		var totalBytes int64
		if err := rows.Scan(&source, &totalBytes); err == nil {
			stats[source] = totalBytes
		}
	}

	bwStatsCache.mu.Lock()
	bwStatsCache.stats = stats
	bwStatsCache.lastFetch = time.Now()
	bwStatsCache.mu.Unlock()

	return stats
}
