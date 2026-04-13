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
	"github.com/relayhq/relay-server/internal/eventstore"
	"github.com/relayhq/relay-server/internal/history"
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
		History:        true,
		HistoryLimit:   100,
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
	h.History = history.NewStore(100)
	h.EventStore = eventstore.NewStore(1000)
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
	if msg.Event != protocol.PusherEventConnectionEstablished {
		ws.Close()
		t.Fatalf("expected %s, got %s", protocol.PusherEventConnectionEstablished, msg.Event)
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

// --- Channel Inspector API Tests ---

func TestChannelsEndpointMapFormat(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/test-key"

	// Connect and subscribe to a channel
	ws := dialAndWaitConnected(t, wsURL)
	defer ws.Close()

	subPayload, _ := json.Marshal(protocol.SubscribeData{Channel: "test-channel"})
	ws.WriteJSON(protocol.Message{Event: protocol.EventSubscribe, Data: subPayload})
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	ws.ReadJSON(&protocol.Message{}) // subscription_succeeded

	// Query channels API
	req, _ := http.NewRequest("GET", baseURL+"/apps/test-app/channels", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	channels, ok := result["channels"].(map[string]any)
	if !ok {
		t.Fatal("expected channels to be a map")
	}

	ch, ok := channels["test-channel"].(map[string]any)
	if !ok {
		t.Fatal("expected test-channel in channels map")
	}

	if ch["type"] != "public" {
		t.Fatalf("expected type public, got %v", ch["type"])
	}
	if ch["subscription_count"] != float64(1) {
		t.Fatalf("expected subscription_count 1, got %v", ch["subscription_count"])
	}
}

func TestChannelsEndpointEmpty(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", baseURL+"/apps/test-app/channels", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	channels, ok := result["channels"].(map[string]any)
	if !ok {
		t.Fatal("expected channels to be a map")
	}
	if len(channels) != 0 {
		t.Fatalf("expected empty channels map, got %d entries", len(channels))
	}
}

func TestChannelEventsEndpoint(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/test-key"
	ws := dialAndWaitConnected(t, wsURL)
	defer ws.Close()

	// Subscribe
	subPayload, _ := json.Marshal(protocol.SubscribeData{Channel: "events-test"})
	ws.WriteJSON(protocol.Message{Event: protocol.EventSubscribe, Data: subPayload})
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	ws.ReadJSON(&protocol.Message{}) // subscription_succeeded

	// Publish 3 events
	for i := 0; i < 3; i++ {
		publishBody, _ := json.Marshal(map[string]any{
			"channel": "events-test",
			"event":   fmt.Sprintf("event-%d", i),
			"data":    map[string]string{"index": fmt.Sprintf("%d", i)},
		})
		req, _ := http.NewRequest("POST", baseURL+"/apps/test-app/events", bytes.NewReader(publishBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("publish failed: %v", err)
		}
		resp.Body.Close()
	}

	// Give the hub time to process events and record history
	time.Sleep(100 * time.Millisecond)

	// Query channel events
	req, _ := http.NewRequest("GET", baseURL+"/apps/test-app/channels/events-test/events?limit=2", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	events, ok := result["events"].([]any)
	if !ok {
		t.Fatal("expected events array")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Should have a next_cursor since we limited to 2 and there are 3
	if result["next_cursor"] == nil {
		t.Fatal("expected next_cursor to be non-nil")
	}

	// Use cursor to get the next page
	cursor := result["next_cursor"].(string)
	req2, _ := http.NewRequest("GET", baseURL+"/apps/test-app/channels/events-test/events?limit=10&cursor="+cursor, nil)
	req2.Header.Set("Authorization", "Bearer test-secret")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()

	var result2 map[string]any
	json.NewDecoder(resp2.Body).Decode(&result2)

	events2, ok := result2["events"].([]any)
	if !ok {
		t.Fatal("expected events array")
	}
	if len(events2) != 1 {
		t.Fatalf("expected 1 event on page 2, got %d", len(events2))
	}
}

func TestAppStatsEndpoint(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	// Connect a client
	wsURL := "ws" + baseURL[4:] + "/app/test-key"
	ws := dialAndWaitConnected(t, wsURL)
	defer ws.Close()

	// Query per-app stats
	req, _ := http.NewRequest("GET", baseURL+"/apps/test-app/stats", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	conns := result["connections"].(float64)
	if conns != 1 {
		t.Fatalf("expected 1 connection, got %v", conns)
	}

	peak := result["peak_connections"].(float64)
	if peak < 1 {
		t.Fatalf("expected peak >= 1, got %v", peak)
	}
}

func TestAppStatsRequiresAuth(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	// No auth header
	req, _ := http.NewRequest("GET", baseURL+"/apps/test-app/stats", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// --- Observability API Tests ---

func TestEventsEndpoint(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/test-key"
	ws := dialAndWaitConnected(t, wsURL)
	defer ws.Close()

	// Subscribe
	subPayload, _ := json.Marshal(protocol.SubscribeData{Channel: "obs-test"})
	ws.WriteJSON(protocol.Message{Event: protocol.EventSubscribe, Data: subPayload})
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	ws.ReadJSON(&protocol.Message{}) // subscription_succeeded

	// Publish events
	for i := 0; i < 3; i++ {
		publishBody, _ := json.Marshal(map[string]any{
			"channel": "obs-test",
			"event":   fmt.Sprintf("test-event-%d", i),
			"data":    map[string]string{"index": fmt.Sprintf("%d", i)},
		})
		req, _ := http.NewRequest("POST", baseURL+"/apps/test-app/events", bytes.NewReader(publishBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("publish failed: %v", err)
		}
		resp.Body.Close()
	}

	// Drain WebSocket messages
	for i := 0; i < 3; i++ {
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		ws.ReadJSON(&protocol.Message{})
	}

	time.Sleep(100 * time.Millisecond)

	// GET /apps/{appId}/events
	req, _ := http.NewRequest("GET", baseURL+"/apps/test-app/events?limit=2", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	events, ok := result["events"].([]any)
	if !ok {
		t.Fatal("expected events array")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Check event structure
	first := events[0].(map[string]any)
	if first["id"] == nil || first["id"] == "" {
		t.Fatal("expected event to have an id")
	}
	if first["channel"] != "obs-test" {
		t.Fatalf("expected channel obs-test, got %v", first["channel"])
	}
	if first["delivered_to"] == nil {
		t.Fatal("expected delivered_to count")
	}

	// Should have a next_cursor
	if result["next_cursor"] == nil {
		t.Fatal("expected next_cursor")
	}
}

func TestEventDetailEndpoint(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/test-key"
	ws := dialAndWaitConnected(t, wsURL)
	defer ws.Close()

	// Subscribe and publish
	subPayload, _ := json.Marshal(protocol.SubscribeData{Channel: "detail-test"})
	ws.WriteJSON(protocol.Message{Event: protocol.EventSubscribe, Data: subPayload})
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	ws.ReadJSON(&protocol.Message{}) // subscription_succeeded

	publishBody, _ := json.Marshal(map[string]any{
		"channel": "detail-test",
		"event":   "detail-event",
		"data":    map[string]string{"key": "value"},
	})
	req, _ := http.NewRequest("POST", baseURL+"/apps/test-app/events", bytes.NewReader(publishBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-secret")
	http.DefaultClient.Do(req)

	// Drain WS
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	ws.ReadJSON(&protocol.Message{})

	time.Sleep(100 * time.Millisecond)

	// Get events list to find the event ID
	req2, _ := http.NewRequest("GET", baseURL+"/apps/test-app/events?limit=1", nil)
	req2.Header.Set("Authorization", "Bearer test-secret")
	resp2, _ := http.DefaultClient.Do(req2)
	var listResult map[string]any
	json.NewDecoder(resp2.Body).Decode(&listResult)
	resp2.Body.Close()

	events := listResult["events"].([]any)
	eventID := events[0].(map[string]any)["id"].(string)

	// GET /apps/{appId}/events/{eventId}
	req3, _ := http.NewRequest("GET", baseURL+"/apps/test-app/events/"+eventID, nil)
	req3.Header.Set("Authorization", "Bearer test-secret")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}

	var detail map[string]any
	json.NewDecoder(resp3.Body).Decode(&detail)

	if detail["id"] != eventID {
		t.Fatalf("expected id %s, got %v", eventID, detail["id"])
	}
	if detail["channel"] != "detail-test" {
		t.Fatalf("expected channel detail-test, got %v", detail["channel"])
	}
	deliveredTo, ok := detail["delivered_to"].([]any)
	if !ok {
		t.Fatal("expected delivered_to to be an array")
	}
	if len(deliveredTo) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveredTo))
	}
}

func TestMetricsEndpoint(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/test-key"
	ws := dialAndWaitConnected(t, wsURL)
	defer ws.Close()

	// Subscribe and publish
	subPayload, _ := json.Marshal(protocol.SubscribeData{Channel: "metrics-test"})
	ws.WriteJSON(protocol.Message{Event: protocol.EventSubscribe, Data: subPayload})
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	ws.ReadJSON(&protocol.Message{}) // subscription_succeeded

	for i := 0; i < 5; i++ {
		publishBody, _ := json.Marshal(map[string]any{
			"channel": "metrics-test",
			"event":   "metric-event",
			"data":    map[string]string{"i": fmt.Sprintf("%d", i)},
		})
		req, _ := http.NewRequest("POST", baseURL+"/apps/test-app/events", bytes.NewReader(publishBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-secret")
		http.DefaultClient.Do(req)
	}

	// Drain WS
	for i := 0; i < 5; i++ {
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		ws.ReadJSON(&protocol.Message{})
	}

	time.Sleep(100 * time.Millisecond)

	// GET /apps/{appId}/metrics
	req, _ := http.NewRequest("GET", baseURL+"/apps/test-app/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var metrics map[string]any
	json.NewDecoder(resp.Body).Decode(&metrics)

	totalEvents := metrics["total_events"].(float64)
	if totalEvents != 5 {
		t.Fatalf("expected 5 total_events, got %v", totalEvents)
	}

	totalDeliveries := metrics["total_deliveries"].(float64)
	if totalDeliveries != 5 {
		t.Fatalf("expected 5 total_deliveries, got %v", totalDeliveries)
	}
}

func TestReplayEndpoint(t *testing.T) {
	baseURL, cleanup := startTestServer(t)
	defer cleanup()

	wsURL := "ws" + baseURL[4:] + "/app/test-key"
	ws := dialAndWaitConnected(t, wsURL)
	defer ws.Close()

	// Subscribe and publish
	subPayload, _ := json.Marshal(protocol.SubscribeData{Channel: "replay-test"})
	ws.WriteJSON(protocol.Message{Event: protocol.EventSubscribe, Data: subPayload})
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	ws.ReadJSON(&protocol.Message{}) // subscription_succeeded

	publishBody, _ := json.Marshal(map[string]any{
		"channel": "replay-test",
		"event":   "replay-event",
		"data":    map[string]string{"msg": "original"},
	})
	req, _ := http.NewRequest("POST", baseURL+"/apps/test-app/events", bytes.NewReader(publishBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-secret")
	http.DefaultClient.Do(req)

	// Drain the original event
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	ws.ReadJSON(&protocol.Message{})

	time.Sleep(100 * time.Millisecond)

	// Get the event ID
	req2, _ := http.NewRequest("GET", baseURL+"/apps/test-app/events?limit=1", nil)
	req2.Header.Set("Authorization", "Bearer test-secret")
	resp2, _ := http.DefaultClient.Do(req2)
	var listResult map[string]any
	json.NewDecoder(resp2.Body).Decode(&listResult)
	resp2.Body.Close()

	events := listResult["events"].([]any)
	eventID := events[0].(map[string]any)["id"].(string)

	// POST /apps/{appId}/events/{eventId}/replay
	req3, _ := http.NewRequest("POST", baseURL+"/apps/test-app/events/"+eventID+"/replay", nil)
	req3.Header.Set("Authorization", "Bearer test-secret")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}

	var replayResult map[string]any
	json.NewDecoder(resp3.Body).Decode(&replayResult)

	if replayResult["ok"] != true {
		t.Fatalf("expected ok=true, got %v", replayResult["ok"])
	}
	if replayResult["new_event_id"] == nil || replayResult["new_event_id"] == "" {
		t.Fatal("expected new_event_id")
	}

	// Should receive the replayed event on WebSocket
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var replayMsg protocol.Message
	if err := ws.ReadJSON(&replayMsg); err != nil {
		t.Fatalf("failed to receive replayed event: %v", err)
	}
	if replayMsg.Event != "replay-event" {
		t.Fatalf("expected event replay-event, got %s", replayMsg.Event)
	}
}
