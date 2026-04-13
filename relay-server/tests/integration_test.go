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
	return startTestServerWithApps(t, &apps.App{
		ID:             "test-app",
		Key:            "test-key",
		Secret:         "test-secret",
		MaxConnections: 100,
	})
}

// startTestServerWithApps starts a Relay server with the given apps on a random port.
func startTestServerWithApps(t *testing.T, appList ...*apps.App) (baseURL string, cleanup func()) {
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
	for _, app := range appList {
		registry.Register(app)
	}

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

// dialAndWaitConnected dials a WebSocket and reads the connection_established message.
// Returns the connection or fails the test.
func dialAndWaitConnected(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg protocol.Message
	if err := ws.ReadJSON(&msg); err != nil {
		ws.Close()
		t.Fatalf("failed to read connection_established: %v", err)
	}
	if msg.Event != protocol.EventConnectionEstablished {
		ws.Close()
		t.Fatalf("expected %s, got %s", protocol.EventConnectionEstablished, msg.Event)
	}
	return ws
}

func TestPublicChannelFlow(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/test-key"
	ws := dialAndWaitConnected(t, wsURL)
	defer ws.Close()

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

func TestConnectionLimitEnforced(t *testing.T) {
	const maxConns = 3

	baseURL, cleanup := startTestServerWithApps(t, &apps.App{
		ID:             "limit-app",
		Key:            "limit-key",
		Secret:         "limit-secret",
		MaxConnections: maxConns,
	})
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/limit-key"

	// Establish max_connections (3) connections successfully
	conns := make([]*websocket.Conn, maxConns)
	for i := 0; i < maxConns; i++ {
		conns[i] = dialAndWaitConnected(t, wsURL)
		defer conns[i].Close()
	}

	// The 4th connection should be rejected with close code 4100
	ws4, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		// Outright connection refusal is acceptable (server closed before upgrade)
		t.Logf("4th connection refused at dial: %v (acceptable)", err)
	} else {
		defer ws4.Close()
		ws4.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, err = ws4.ReadMessage()
		if err == nil {
			t.Fatal("expected 4th connection to be closed, but it received a message")
		}
		closeErr, ok := err.(*websocket.CloseError)
		if ok && closeErr.Code != 4100 {
			t.Fatalf("expected close code 4100, got %d", closeErr.Code)
		}
	}

	// Close one connection to free a slot
	conns[0].Close()
	// Give the server time to process the disconnect
	time.Sleep(200 * time.Millisecond)

	// A new connection should now succeed
	ws5 := dialAndWaitConnected(t, wsURL)
	defer ws5.Close()
}

func TestConnectionLimitPerApp(t *testing.T) {
	// Verify that App A being at its limit does not affect App B
	baseURL, cleanup := startTestServerWithApps(t,
		&apps.App{
			ID:             "app-a",
			Key:            "key-a",
			Secret:         "secret-a",
			MaxConnections: 1,
		},
		&apps.App{
			ID:             "app-b",
			Key:            "key-b",
			Secret:         "secret-b",
			MaxConnections: 1,
		},
	)
	defer cleanup()

	// Fill App A's single slot
	wsA := dialAndWaitConnected(t, "ws"+baseURL[4:]+"/app/key-a")
	defer wsA.Close()

	// App B should still accept a connection
	wsB := dialAndWaitConnected(t, "ws"+baseURL[4:]+"/app/key-b")
	defer wsB.Close()
}

func TestConnectionLimitUnlimited(t *testing.T) {
	// max_connections=0 means unlimited
	baseURL, cleanup := startTestServerWithApps(t, &apps.App{
		ID:             "unlimited-app",
		Key:            "unlimited-key",
		Secret:         "unlimited-secret",
		MaxConnections: 0,
	})
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/unlimited-key"

	// Should be able to connect many times without rejection
	for i := 0; i < 5; i++ {
		ws := dialAndWaitConnected(t, wsURL)
		defer ws.Close()
	}
}
