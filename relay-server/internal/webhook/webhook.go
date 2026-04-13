package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/relayhq/relay-server/internal/apps"
)

// Payload is the JSON body sent to webhook endpoints.
type Payload struct {
	Event     string `json:"event"`
	Channel   string `json:"channel"`
	AppID     string `json:"app_id"`
	Timestamp int64  `json:"timestamp"`
	UserID    any    `json:"user_id,omitempty"`
	UserInfo  any    `json:"user_info,omitempty"`
}

// Dispatcher fires webhook HTTP requests for app lifecycle events.
type Dispatcher struct {
	client *http.Client
}

// NewDispatcher creates a webhook dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Fire sends a webhook event to all matching endpoints for the given app.
// It runs asynchronously and never blocks the caller.
func (d *Dispatcher) Fire(app *apps.App, event, channel string, extra map[string]any) {
	if len(app.Webhooks) == 0 {
		return
	}

	payload := Payload{
		Event:     event,
		Channel:   channel,
		AppID:     app.ID,
		Timestamp: time.Now().Unix(),
	}
	if v, ok := extra["user_id"]; ok {
		payload.UserID = v
	}
	if v, ok := extra["user_info"]; ok {
		payload.UserInfo = v
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	sig := sign(app.Secret, body)

	for _, wh := range app.Webhooks {
		if !matchesEvent(wh.Events, event) {
			continue
		}
		go d.send(wh.URL, body, sig, app.ID)
	}
}

// send delivers a webhook with up to 3 retries and exponential backoff.
func (d *Dispatcher) send(url string, body []byte, signature, appID string) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
		}

		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			slog.Error("webhook request error",
				"app_id", appID,
				"error", err.Error())
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Relay-Signature", signature)

		resp, err := d.client.Do(req)
		if err != nil {
			slog.Warn("webhook delivery failed",
				"app_id", appID,
				"attempt", attempt+1,
				"error", err.Error())
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return
		}
		slog.Warn("webhook returned non-2xx",
			"app_id", appID,
			"status", resp.StatusCode,
			"attempt", attempt+1)
	}
	slog.Error("webhook delivery failed after retries",
		"app_id", appID,
		"url", url,
		"attempts", 3)
}

// sign computes HMAC-SHA256 of the body using the app secret.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// matchesEvent checks if the event is in the subscribed list.
func matchesEvent(subscribed []string, event string) bool {
	for _, e := range subscribed {
		if e == event {
			return true
		}
	}
	return false
}
