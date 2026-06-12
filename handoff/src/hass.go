package main
import "log"
func NotifyHomeAssistant(state string) {
	log.Printf("🏠 Notifying Home Assistant of stream state: %s", state)
}
