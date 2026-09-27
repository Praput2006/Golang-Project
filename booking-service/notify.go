package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

// notify calls Notification Service's POST /notifications endpoint to
// record a BOOKING_* event. This stands in for a real event bus (Kafka/
// RabbitMQ) for this project: it's a direct, best-effort HTTP call, so a
// down or not-yet-built Notification Service never blocks a booking action.
// Call it with `go notify(...)` from handlers so it never adds latency.
func notify(userID uint, notifType, title, message string) {
	baseURL := os.Getenv("NOTIFICATION_SERVICE_URL")
	if baseURL == "" {
		return // Notification Service not wired up yet; skip silently.
	}

	payload := map[string]interface{}{
		"user_id": userID,
		"type":    notifType,
		"title":   title,
		"message": message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("notify: failed to encode payload: %v", err)
		return
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(baseURL+"/notifications", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("notify: notification service unreachable: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("notify: notification service returned status %d", resp.StatusCode)
	}
}
