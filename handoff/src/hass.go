package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func NotifyHomeAssistant(state string, targetLink string) {
	hassURL := os.Getenv("HASS_WEBHOOK_URL")
	if hassURL == "" {
		return
	}

	payload := map[string]string{
		"state": state,
		"link":  targetLink,
	}
	body, _ := json.Marshal(payload)

	go func() {
		resp, err := http.Post(hassURL, "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Printf("⚠️ Home Assistant notification failed: %v", err)
			return
		}
		defer resp.Body.Close()
		log.Printf("🏠 Home Assistant notified of state: %s", state)
	}()
}
