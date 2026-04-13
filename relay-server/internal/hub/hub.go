package hub

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/relayhq/relay-server/internal/apps"
	"github.com/relayhq/relay-server/internal/auth"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/history"
	"github.com/relayhq/relay-server/internal/protocol"
	"github.com/relayhq/relay-server/internal/webhook"
)

// incomingMessage wraps a message with its sender for routing.
type incomingMessage struct {
	client  *Client
	message *protocol.Message
}

// Hub is the central broker for all WebSocket connections and channels.
// It manages the lifecycle of every client connection and routes all messages.
// The hub runs a single goroutine event loop (Run) to avoid race conditions.
type Hub struct {
	mu sync.RWMutex

	// All currently connected clients (key: socket ID)
	clients map[string]*Client

	// All active channels (key: appID + "\x00" + channelName)
	channels map[string]*Channel

	// Config reference
	cfg *config.Config

	// App registry
	registry *apps.AppRegistry

	// Inbound channels for the event loop
	register   chan *Client
	unregister chan *Client
	incoming   chan *incomingMessage

	// Publish from HTTP API
	Publish chan *protocol.PublishRequest

	// Event log ring buffer
	eventLog    [100]EventLogEntry
	eventLogPos int
	eventLogLen int

	// History store for channel event replay
	History *history.Store

	// Webhook dispatcher
	Webhooks *webhook.Dispatcher

	// Per-app connection counts for limit enforcement.
	// Atomic counters allow the WebSocket handler to check limits
	// without going through the single-threaded event loop.
	appConnMu     sync.Mutex
	appConnCounts map[string]*atomic.Int64

	// Per-app peak connections (high-water mark)
	appPeakMu     sync.Mutex
	appPeakConns  map[string]*atomic.Int64

	// Per-app message counters (total messages since startup)
	appMsgMu     sync.Mutex
	appMsgCounts map[string]*atomic.Int64
}

// EventLogEntry records a published event for the dashboard.
type EventLogEntry struct {
	Timestamp string `json:"timestamp"`
	Channel   string `json:"channel"`
	Event     string `json:"event"`
	AppID     string `json:"app_id"`
}

// channelKey builds a composite key for the channels map.
func channelKey(appID, channelName string) string {
	return appID + "\x00" + channelName
}

// NewHub creates a Hub ready to be started with Run().
func NewHub(cfg *config.Config, registry *apps.AppRegistry) *Hub {
	return &Hub{
		clients:       make(map[string]*Client),
		channels:      make(map[string]*Channel),
		cfg:           cfg,
		registry:      registry,
		register:      make(chan *Client, 256),
		unregister:    make(chan *Client, 256),
		incoming:      make(chan *incomingMessage, 1024),
		Publish:       make(chan *protocol.PublishRequest, 1024),
		appConnCounts: make(map[string]*atomic.Int64),
		appPeakConns:  make(map[string]*atomic.Int64),
		appMsgCounts:  make(map[string]*atomic.Int64),
	}
}

// --- Per-App Connection Limit Enforcement ---

// getAppCounter returns the atomic counter for an app, creating it if needed.
func (h *Hub) getAppCounter(appID string) *atomic.Int64 {
	h.appConnMu.Lock()
	counter, ok := h.appConnCounts[appID]
	if !ok {
		counter = &atomic.Int64{}
		h.appConnCounts[appID] = counter
	}
	h.appConnMu.Unlock()
	return counter
}

// TryIncrementConns atomically checks whether the app is under its connection
// limit and increments the counter if so. Returns (currentCount, true) on
// success, or (currentCount, false) if the limit has been reached.
// If maxConns is 0, the limit is treated as unlimited.
func (h *Hub) TryIncrementConns(appID string, maxConns int) (int64, bool) {
	counter := h.getAppCounter(appID)
	for {
		current := counter.Load()
		if maxConns > 0 && current >= int64(maxConns) {
			return current, false
		}
		if counter.CompareAndSwap(current, current+1) {
			newCount := current + 1
			// Update peak connection high-water mark
			peak := h.getAppPeak(appID)
			for {
				curPeak := peak.Load()
				if newCount <= curPeak {
					break
				}
				if peak.CompareAndSwap(curPeak, newCount) {
					break
				}
			}
			return newCount, true
		}
	}
}

// getAppPeak returns the peak connection counter for an app.
func (h *Hub) getAppPeak(appID string) *atomic.Int64 {
	h.appPeakMu.Lock()
	peak, ok := h.appPeakConns[appID]
	if !ok {
		peak = &atomic.Int64{}
		h.appPeakConns[appID] = peak
	}
	h.appPeakMu.Unlock()
	return peak
}

// AppPeakConnCount returns the peak connection count for an app.
func (h *Hub) AppPeakConnCount(appID string) int64 {
	return h.getAppPeak(appID).Load()
}

// getAppMsgCounter returns the message counter for an app.
func (h *Hub) getAppMsgCounter(appID string) *atomic.Int64 {
	h.appMsgMu.Lock()
	counter, ok := h.appMsgCounts[appID]
	if !ok {
		counter = &atomic.Int64{}
		h.appMsgCounts[appID] = counter
	}
	h.appMsgMu.Unlock()
	return counter
}

// IncrementMsgCount increments the message counter for an app.
func (h *Hub) IncrementMsgCount(appID string) {
	h.getAppMsgCounter(appID).Add(1)
}

// AppMsgCount returns the total message count for an app since startup.
func (h *Hub) AppMsgCount(appID string) int64 {
	return h.getAppMsgCounter(appID).Load()
}

// DecrementConns decrements the per-app connection counter.
// Must be called exactly once for each successful TryIncrementConns call.
func (h *Hub) DecrementConns(appID string) {
	counter := h.getAppCounter(appID)
	counter.Add(-1)
}

// AppConnCount returns the current connection count for an app.
func (h *Hub) AppConnCount(appID string) int64 {
	counter := h.getAppCounter(appID)
	return counter.Load()
}

// Run starts the hub's event loop. This should be called in its own goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.handleRegister(client)

		case client := <-h.unregister:
			h.handleUnregister(client)

		case msg := <-h.incoming:
			h.handleIncoming(msg)

		case req := <-h.Publish:
			h.handlePublish(req)
		}
	}
}

// --- Event Loop Handlers ---

func (h *Hub) handleRegister(client *Client) {
	h.mu.Lock()
	h.clients[client.SocketID] = client
	count := len(h.clients)
	h.mu.Unlock()

	slog.Debug("client connected",
		"socket_id", client.SocketID,
		"app_id", client.app.ID,
		"connections", count)

	if err := client.sendConnectionEstablished(); err != nil {
		slog.Error("failed to send connection_established",
			"socket_id", client.SocketID,
			"error", err.Error())
	}
}

func (h *Hub) handleUnregister(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client.SocketID]; !ok {
		h.mu.Unlock()
		return
	}
	delete(h.clients, client.SocketID)
	close(client.send)
	h.mu.Unlock()

	h.mu.Lock()
	for channelName := range client.subscriptions {
		key := channelKey(client.app.ID, channelName)
		ch, ok := h.channels[key]
		if !ok {
			continue
		}
		member := ch.Unsubscribe(client)

		if ch.Type == protocol.ChannelTypePresence && member != nil {
			go h.broadcastMemberRemoved(ch, member)
			h.fireWebhook(client.app, "member.removed", channelName, map[string]any{
				"user_id": member.ID, "user_info": member.Info,
			})
		}

		if ch.IsEmpty() {
			delete(h.channels, key)
			h.fireWebhook(client.app, "channel.vacated", channelName, nil)
		}
	}
	h.mu.Unlock()

	slog.Debug("client disconnected", "socket_id", client.SocketID, "app_id", client.app.ID)
}

func (h *Hub) handleIncoming(im *incomingMessage) {
	msg := im.message
	client := im.client

	slog.Debug("incoming message",
		"socket_id", client.SocketID,
		"event", msg.Event,
		"channel", msg.Channel)

	switch msg.Event {
	case protocol.EventSubscribe, protocol.PusherEventSubscribe:
		h.handleSubscribe(client, msg)

	case protocol.EventUnsubscribe, protocol.PusherEventUnsubscribe:
		h.handleUnsubscribe(client, msg)

	case protocol.EventPing, protocol.PusherEventPing:
		h.handlePing(client)

	default:
		if len(msg.Event) > 7 && msg.Event[:7] == "client-" {
			h.handleClientEvent(client, msg)
		}
	}
}

func (h *Hub) handleSubscribe(client *Client, msg *protocol.Message) {
	var subData protocol.SubscribeData
	if err := json.Unmarshal(msg.Data, &subData); err != nil {
		h.sendError(client, 4000, "Invalid subscribe payload")
		return
	}

	channelName := subData.Channel
	if channelName == "" {
		h.sendError(client, 4000, "Channel name is required")
		return
	}

	if len(channelName) > h.cfg.MaxChannelNameLength {
		h.sendError(client, 4009, "Channel name too long")
		return
	}

	channelType := protocol.ChannelTypeFromName(channelName)

	if channelType == protocol.ChannelTypePrivate || channelType == protocol.ChannelTypePresence {
		channelData := ""
		if channelType == protocol.ChannelTypePresence {
			channelData = subData.ChannelData
		}
		if !h.validateAuth(client.app.Key, client.app.Secret, client.SocketID, channelName, subData.Auth, channelData) {
			h.sendError(client, 4009, "Invalid authentication signature")
			return
		}
	}

	key := channelKey(client.app.ID, channelName)
	h.mu.Lock()
	ch, ok := h.channels[key]
	isNewChannel := false
	if !ok {
		ch = newChannel(channelName)
		h.channels[key] = ch
		isNewChannel = true
	}
	h.mu.Unlock()

	if err := ch.Subscribe(client, subData.ChannelData); err != nil {
		h.sendError(client, 4000, err.Error())
		return
	}
	client.subscriptions[channelName] = true

	// Fire channel.occupied webhook on first subscriber
	if isNewChannel {
		h.fireWebhook(client.app, "channel.occupied", channelName, nil)
	}

	var successData any = map[string]any{}
	if channelType == protocol.ChannelTypePresence {
		successData = ch.PresenceData()
	}

	successMsg, err := protocol.NewMessage(protocol.EventSubscriptionSucceeded, channelName, successData)
	if err != nil {
		slog.Error("failed to build subscription_succeeded",
			"error", err.Error())
		return
	}
	client.SendMessage(successMsg)

	// Replay history if last_event_id was provided and app has history enabled
	if subData.LastEventID > 0 && h.History != nil && client.app.History {
		events := h.History.GetAfterID(client.app.ID, channelName, subData.LastEventID, client.app.HistoryLimit)
		for _, e := range events {
			replayMsg := &protocol.Message{
				Event:   e.EventName,
				Channel: channelName,
				Data:    e.Data,
			}
			client.SendMessage(replayMsg)
		}
	}

	if channelType == protocol.ChannelTypePresence && subData.ChannelData != "" {
		var member protocol.PresenceMember
		if err := json.Unmarshal([]byte(subData.ChannelData), &member); err == nil {
			go h.broadcastMemberAdded(ch, client.SocketID, &member)
			h.fireWebhook(client.app, "member.added", channelName, map[string]any{
				"user_id": member.ID, "user_info": member.Info,
			})
		}
	}

	slog.Debug("client subscribed",
		"socket_id", client.SocketID,
		"channel", channelName,
		"app_id", client.app.ID)
}

func (h *Hub) handleUnsubscribe(client *Client, msg *protocol.Message) {
	var subData protocol.SubscribeData
	if err := json.Unmarshal(msg.Data, &subData); err != nil {
		return
	}

	channelName := subData.Channel
	key := channelKey(client.app.ID, channelName)
	h.mu.Lock()
	ch, ok := h.channels[key]
	h.mu.Unlock()

	if !ok {
		return
	}

	member := ch.Unsubscribe(client)
	delete(client.subscriptions, channelName)

	if ch.Type == protocol.ChannelTypePresence && member != nil {
		go h.broadcastMemberRemoved(ch, member)
		h.fireWebhook(client.app, "member.removed", channelName, map[string]any{
			"user_id": member.ID, "user_info": member.Info,
		})
	}

	h.mu.Lock()
	if ch.IsEmpty() {
		delete(h.channels, key)
		h.fireWebhook(client.app, "channel.vacated", channelName, nil)
	}
	h.mu.Unlock()
}

func (h *Hub) handlePing(client *Client) {
	pong, _ := protocol.NewMessage(protocol.EventPong, "", nil)
	client.SendMessage(pong)
}

func (h *Hub) handleClientEvent(client *Client, msg *protocol.Message) {
	if msg.Channel == "" {
		return
	}

	key := channelKey(client.app.ID, msg.Channel)
	h.mu.RLock()
	ch, ok := h.channels[key]
	h.mu.RUnlock()

	if !ok {
		return
	}

	if !client.subscriptions[msg.Channel] {
		return
	}

	if ch.Type == protocol.ChannelTypePublic {
		return
	}

	ch.Broadcast(msg, client.SocketID)
}

func (h *Hub) handlePublish(req *protocol.PublishRequest) {
	key := channelKey(req.AppID, req.Channel)
	h.mu.RLock()
	ch, ok := h.channels[key]
	h.mu.RUnlock()

	if !ok {
		return
	}

	msg := &protocol.Message{
		Event:   req.Event,
		Channel: req.Channel,
		Data:    req.Data,
	}

	ch.Broadcast(msg, req.SocketID)

	// Increment per-app message counter
	h.IncrementMsgCount(req.AppID)

	// Record to history if enabled for this app
	if h.History != nil {
		app, ok := h.registry.LookupByID(req.AppID)
		if ok && app.History {
			h.History.Record(req.AppID, req.Channel, req.Event, req.Data, req.SocketID, app.HistoryLimit)
		}
	}

	// Record to event log ring buffer
	h.mu.Lock()
	h.eventLog[h.eventLogPos] = EventLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Channel:   req.Channel,
		Event:     req.Event,
		AppID:     req.AppID,
	}
	h.eventLogPos = (h.eventLogPos + 1) % len(h.eventLog)
	if h.eventLogLen < len(h.eventLog) {
		h.eventLogLen++
	}
	h.mu.Unlock()

	slog.Debug("published event",
		"event", req.Event,
		"channel", req.Channel,
		"app_id", req.AppID)
}

// --- Presence Helpers ---

func (h *Hub) broadcastMemberAdded(ch *Channel, excludeSocketID string, member *protocol.PresenceMember) {
	msg, err := protocol.NewMessage(protocol.EventMemberAdded, ch.Name, member)
	if err != nil {
		return
	}
	ch.Broadcast(msg, excludeSocketID)
}

func (h *Hub) broadcastMemberRemoved(ch *Channel, member *protocol.PresenceMember) {
	msg, err := protocol.NewMessage(protocol.EventMemberRemoved, ch.Name, member)
	if err != nil {
		return
	}
	ch.Broadcast(msg, "")
}

// --- Webhook Helper ---

func (h *Hub) fireWebhook(app *apps.App, event, channel string, extra map[string]any) {
	if h.Webhooks != nil {
		go h.Webhooks.Fire(app, event, channel, extra)
	}
}

// --- Auth Validation ---

func (h *Hub) validateAuth(appKey, appSecret, socketID, channelName, authToken, channelData string) bool {
	return auth.Validate(appKey, appSecret, socketID, channelName, authToken, channelData)
}

// --- Error Handling ---

func (h *Hub) sendError(client *Client, code int, message string) {
	errMsg, err := protocol.NewMessage(protocol.EventError, "", protocol.ErrorData{
		Code:    code,
		Message: message,
	})
	if err != nil {
		return
	}
	client.SendMessage(errMsg)
}

// --- Public API for HTTP handlers ---

// GetChannels returns info about all active channels for the given app.
func (h *Hub) GetChannels(appID string) []ChannelInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	prefix := appID + "\x00"
	infos := make([]ChannelInfo, 0)
	for key, ch := range h.channels {
		if strings.HasPrefix(key, prefix) {
			info := ch.Info()
			info.AppID = appID
			infos = append(infos, info)
		}
	}
	return infos
}

// GetAllChannels returns info about all active channels across all apps.
func (h *Hub) GetAllChannels() []ChannelInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	infos := make([]ChannelInfo, 0, len(h.channels))
	for key, ch := range h.channels {
		info := ch.Info()
		idx := strings.IndexByte(key, '\x00')
		if idx >= 0 {
			info.AppID = key[:idx]
		}
		infos = append(infos, info)
	}
	return infos
}

// GetChannel returns info about a specific channel, or nil if not found.
func (h *Hub) GetChannel(appID, name string) *ChannelInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ch, ok := h.channels[channelKey(appID, name)]
	if !ok {
		return nil
	}
	info := ch.Info()
	info.AppID = appID
	return &info
}

// GetChannelMembers returns presence members for a channel.
func (h *Hub) GetChannelMembers(appID, name string) map[string]*protocol.PresenceMember {
	h.mu.RLock()
	ch, ok := h.channels[channelKey(appID, name)]
	h.mu.RUnlock()

	if !ok {
		return nil
	}
	return ch.GetMembers()
}

// ConnectionCount returns the total number of connected clients.
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ChannelCount returns the total number of active channels.
func (h *Hub) ChannelCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.channels)
}

// AppStats returns per-app connection and channel counts.
func (h *Hub) AppStats() []map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()

	connCounts := make(map[string]int)
	for _, c := range h.clients {
		connCounts[c.app.ID]++
	}

	chanCounts := make(map[string]int)
	for key := range h.channels {
		idx := strings.IndexByte(key, '\x00')
		if idx >= 0 {
			chanCounts[key[:idx]]++
		}
	}

	allApps := h.registry.All()
	stats := make([]map[string]any, 0, len(allApps))
	for _, app := range allApps {
		stats = append(stats, map[string]any{
			"id":          app.ID,
			"key":         app.Key,
			"connections": connCounts[app.ID],
			"channels":    chanCounts[app.ID],
		})
	}
	return stats
}

// GetEventLog returns the last n events from the ring buffer in reverse chronological order.
// If appID is non-empty, only events for that app are returned.
func (h *Hub) GetEventLog(n int, appID string) []EventLogEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]EventLogEntry, 0, n)
	for i := 0; i < h.eventLogLen && len(result) < n; i++ {
		idx := (h.eventLogPos - 1 - i + len(h.eventLog)) % len(h.eventLog)
		entry := h.eventLog[idx]
		if appID == "" || entry.AppID == appID {
			result = append(result, entry)
		}
	}
	return result
}

// Shutdown broadcasts a shutdown error to all connected clients,
// waits 1 second for delivery, then closes all client send channels.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()

	errMsg, err := protocol.NewMessage(protocol.EventError, "", protocol.ErrorData{
		Code:    4200,
		Message: "Server shutting down",
	})
	if err != nil {
		return
	}
	data, err := errMsg.Encode()
	if err != nil {
		return
	}

	for _, client := range h.clients {
		select {
		case client.send <- data:
		default:
		}
	}

	time.Sleep(1 * time.Second)

	for socketID, client := range h.clients {
		close(client.send)
		delete(h.clients, socketID)
	}
}

// RegisterClient is called by the WebSocket handler to register a new connection.
func (h *Hub) RegisterClient(client *Client) {
	h.register <- client
}

// UnregisterClient is called when a WebSocket connection closes.
func (h *Hub) UnregisterClient(client *Client) {
	h.unregister <- client
}
