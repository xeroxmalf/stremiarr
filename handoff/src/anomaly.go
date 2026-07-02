package main

import (
	"fmt"
	"log"
	"time"
)

// DetectAnomalies runs periodically to check if stream fail rates are unusually high
func DetectAnomalies() {
	if db == nil {
		return
	}

	var totalStreams int
	var highFailStreams int

	// Count total streams
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls").Scan(&totalStreams); err != nil {
		log.Printf("⚠️ Failed to count streams: %v", err)
		return
	}

	// Count streams that have failed 3 or more times (dead links)
	if err := db.QueryRow("SELECT COUNT(*) FROM stream_urls WHERE fail_count >= 3").Scan(&highFailStreams); err != nil {
		log.Printf("⚠️ Failed to count high failure streams: %v", err)
		return
	}

	if totalStreams == 0 {
		return
	}

	failureRate := float64(highFailStreams) / float64(totalStreams)
	log.Printf("🚨 Anomaly Detection: Stream failure rate is %.2f%%", failureRate*100)

	// If more than 20% of streams are dead, trigger an anomaly webhook alert!
	if failureRate > 0.20 {
		log.Printf("⚠️ ANOMALY DETECTED: High stream failure rate!")
		FireWebhook("anomaly_detected", fmt.Sprintf("High stream failure rate detected: %.2f%% of all known streams are returning errors.", failureRate*100))
	}
}

// StartAnomalyWorker runs the anomaly detection in the background every hour
func StartAnomalyWorker() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			DetectAnomalies()
		}
	}()
}
