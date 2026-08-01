package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
)

type WebhookPayload struct {
	Event   string `json:"event"`
	Message string `json:"message"`
}

func FireWebhook(event, message string) {
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		return
	}

	payload := WebhookPayload{
		Event:   event,
		Message: message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("⚠️ Webhook marshal failed: %v", err)
		return
	}

	go func() {
		resp, err := httpClient.Post(webhookURL, "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Printf("⚠️ Webhook delivery failed: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			log.Printf("⚠️ Webhook delivery returned HTTP %d", resp.StatusCode)
		}
	}()
}
