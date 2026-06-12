package main

import (
	"log"
)

// TrackBandwidth logs proxied bytes and API payloads into the database
func TrackBandwidth(bytes int, source string) {
	if db == nil {
		return
	}
	_, err := db.Exec("INSERT INTO bandwidth_log (bytes, source) VALUES (?, ?)", bytes, source)
	if err != nil {
		log.Printf("⚠️ Failed to log bandwidth: %v", err)
	}
	TrackMetrics(int64(bytes))
	log.Printf("📈 Tracked %d bytes of bandwidth from source: %s", bytes, source)
}

// GetBandwidthStats returns the total bandwidth consumed grouped by source.
func GetBandwidthStats() map[string]int64 {
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
	return stats
}
