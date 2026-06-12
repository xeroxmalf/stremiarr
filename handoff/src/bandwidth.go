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
		log.Printf("⚠️ Failed to log bandwidth usage: %v", err)
		return
	}
	log.Printf("📈 Tracked %d bytes of bandwidth from source: %s", bytes, source)
}
