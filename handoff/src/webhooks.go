package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
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
	body, _ := json.Marshal(payload)
	go func() {
		resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Printf("⚠️ Webhook delivery failed: %v", err)
			return
		}
		defer resp.Body.Close()
	}()
}
