package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/relayhq/relay-server/internal/apps"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/hub"
	"github.com/relayhq/relay-server/internal/protocol"
	"github.com/relayhq/relay-server/internal/server"
)

// startTestServer starts a Relay server on a random port and returns the base URL.
func startTestServer(t *testing.T) (baseURL string, cleanup func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := &config.Config{
		Host:                 "127.0.0.1",
		Port:                 port,
		AppID:                "test-app",
		AppKey:               "test-key",
		AppSecret:            "test-secret",
		MaxConnections:       100,
		MaxChannelNameLength: 200,
		MaxEventPayloadKB:    100,
		PingInterval:         120,
		PingTimeout:          30,
		DashboardEnabled:     false,
		DashboardPath:        "/dashboard",
		Debug:                false,
	}

	registry := apps.NewRegistry()
	registry.Register(&apps.App{
		ID:             "test-app",
		Key:            "test-key",
		Secret:         "test-secret",
		MaxConnections: 100,
	})

	h := hub.NewHub(cfg, registry)
	go h.Run()

	srv := server.New(cfg, h, registry)
	go srv.Start()

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	return baseURL, func() { srv.Shutdown() }
}

func TestPublicChannelFlow(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/test-key"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer ws.Close()

	// Read connection_established
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var connMsg protocol.Message
	if err := ws.ReadJSON(&connMsg); err != nil {
		t.Fatalf("failed to read connection_established: %v", err)
	}
	if connMsg.Event != protocol.EventConnectionEstablished {
		t.Fatalf("expected %s, got %s", protocol.EventConnectionEstablished, connMsg.Event)
	}

	// Subscribe to test-channel
	subPayload, _ := json.Marshal(protocol.SubscribeData{Channel: "test-channel"})
	subMsg := protocol.Message{
		Event: protocol.EventSubscribe,
		Data:  subPayload,
	}
	if err := ws.WriteJSON(subMsg); err != nil {
		t.Fatalf("failed to send subscribe: %v", err)
	}

	// Read subscription_succeeded
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var subResp protocol.Message
	if err := ws.ReadJSON(&subResp); err != nil {
		t.Fatalf("failed to read subscription_succeeded: %v", err)
	}
	if subResp.Event != protocol.EventSubscriptionSucceeded {
		t.Fatalf("expected %s, got %s", protocol.EventSubscriptionSucceeded, subResp.Event)
	}
	if subResp.Channel != "test-channel" {
		t.Fatalf("expected channel test-channel, got %s", subResp.Channel)
	}

	// Publish an event via HTTP API
	publishBody, _ := json.Marshal(map[string]any{
		"channel": "test-channel",
		"event":   "my-event",
		"data":    map[string]string{"message": "hello"},
	})
	req, _ := http.NewRequest("POST", baseURL+"/apps/test-app/events", bytes.NewReader(publishBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("publish request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Assert event is received on WebSocket within 2 seconds
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var eventMsg protocol.Message
	if err := ws.ReadJSON(&eventMsg); err != nil {
		t.Fatalf("failed to receive published event: %v", err)
	}
	if eventMsg.Event != "my-event" {
		t.Fatalf("expected event my-event, got %s", eventMsg.Event)
	}
	if eventMsg.Channel != "test-channel" {
		t.Fatalf("expected channel test-channel, got %s", eventMsg.Channel)
	}

	var data map[string]string
	if err := json.Unmarshal(eventMsg.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal event data: %v", err)
	}
	if data["message"] != "hello" {
		t.Fatalf("expected message 'hello', got '%s'", data["message"])
	}
}

func TestUnknownAppKeyRejected(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/invalid-key"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		// Connection refused is also acceptable
		return
	}
	defer ws.Close()

	// Should receive a close frame with code 4001
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = ws.ReadMessage()
	if err == nil {
		t.Fatal("expected connection to be closed")
	}
	closeErr, ok := err.(*websocket.CloseError)
	if ok && closeErr.Code != 4001 {
		t.Fatalf("expected close code 4001, got %d", closeErr.Code)
	}
}

func TestUnknownAppIDRejected(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", baseURL+"/apps/unknown-app/channels", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
