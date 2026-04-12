package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/relayhq/relay-server/internal/auth"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/protocol"
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

	// All active channels (key: channel name)
	channels map[string]*Channel

	// Config reference
	cfg *config.Config

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
}

// EventLogEntry records a published event for the dashboard.
type EventLogEntry struct {
	Timestamp string `json:"timestamp"`
	Channel   string `json:"channel"`
	Event     string `json:"event"`
}

// NewHub creates a Hub ready to be started with Run().
func NewHub(cfg *config.Config) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		channels:   make(map[string]*Channel),
		cfg:        cfg,
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
		incoming:   make(chan *incomingMessage, 1024),
		Publish:    make(chan *protocol.PublishRequest, 1024),
	}
}

// Run starts the hub's event loop. This should be called in its own goroutine.
// It processes all register/unregister/message events sequentially,
// avoiding the need for most locking.
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

// NewClient creates a new client, registers it, and returns it.
// Called from the WebSocket upgrade handler.
func (h *Hub) NewClient(conn interface{}, wsConn interface{}) *Client {
	// This is called from websocket handler — see websocket/handler.go
	return nil
}

// --- Event Loop Handlers ---

func (h *Hub) handleRegister(client *Client) {
	h.mu.Lock()
	h.clients[client.SocketID] = client
	count := len(h.clients)
	h.mu.Unlock()

	if h.cfg.Debug {
		log.Printf("[Relay] Client connected: %s (total: %d)", client.SocketID, count)
	}

	// Send the connection established event
	if err := client.sendConnectionEstablished(); err != nil {
		log.Printf("[Relay] Error sending connection_established to %s: %v", client.SocketID, err)
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

	// Unsubscribe from all channels and broadcast presence removals
	h.mu.Lock()
	for channelName := range client.subscriptions {
		ch, ok := h.channels[channelName]
		if !ok {
			continue
		}
		member := ch.Unsubscribe(client)

		// Broadcast member_removed to presence channels
		if ch.Type == protocol.ChannelTypePresence && member != nil {
			go h.broadcastMemberRemoved(ch, member)
		}

		// Clean up empty channels
		if ch.IsEmpty() {
			delete(h.channels, channelName)
		}
	}
	h.mu.Unlock()

	if h.cfg.Debug {
		log.Printf("[Relay] Client disconnected: %s", client.SocketID)
	}
}

func (h *Hub) handleIncoming(im *incomingMessage) {
	msg := im.message
	client := im.client

	if h.cfg.Debug {
		log.Printf("[Relay] Incoming from %s: event=%s channel=%s", client.SocketID, msg.Event, msg.Channel)
	}

	switch msg.Event {
	case protocol.EventSubscribe, protocol.PusherEventSubscribe:
		h.handleSubscribe(client, msg)

	case protocol.EventUnsubscribe, protocol.PusherEventUnsubscribe:
		h.handleUnsubscribe(client, msg)

	case protocol.EventPing, protocol.PusherEventPing:
		h.handlePing(client)

	default:
		// Client events (prefixed with "client-") are forwarded to the channel
		if len(msg.Event) > 7 && msg.Event[:7] == "client-" {
			h.handleClientEvent(client, msg)
		}
	}
}

func (h *Hub) handleSubscribe(client *Client, msg *protocol.Message) {
	// Parse subscribe payload
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

	// Validate auth for private and presence channels
	if channelType == protocol.ChannelTypePrivate || channelType == protocol.ChannelTypePresence {
		channelData := ""
		if channelType == protocol.ChannelTypePresence {
			channelData = subData.ChannelData
		}
		if !h.validateAuth(client.SocketID, channelName, subData.Auth, channelData) {
			h.sendError(client, 4009, "Invalid authentication signature")
			return
		}
	}

	// Get or create the channel
	h.mu.Lock()
	ch, ok := h.channels[channelName]
	if !ok {
		ch = newChannel(channelName)
		h.channels[channelName] = ch
	}
	h.mu.Unlock()

	// Subscribe the client
	if err := ch.Subscribe(client, subData.ChannelData); err != nil {
		h.sendError(client, 4000, err.Error())
		return
	}
	client.subscriptions[channelName] = true

	// Build subscription_succeeded response
	var successData any = map[string]any{}
	if channelType == protocol.ChannelTypePresence {
		successData = ch.PresenceData()
	}

	successMsg, err := protocol.NewMessage(protocol.EventSubscriptionSucceeded, channelName, successData)
	if err != nil {
		log.Printf("[Relay] Error building subscription_succeeded: %v", err)
		return
	}
	client.SendMessage(successMsg)

	// For presence channels, notify existing members of the new arrival
	if channelType == protocol.ChannelTypePresence && subData.ChannelData != "" {
		var member protocol.PresenceMember
		if err := json.Unmarshal([]byte(subData.ChannelData), &member); err == nil {
			go h.broadcastMemberAdded(ch, client.SocketID, &member)
		}
	}

	if h.cfg.Debug {
		log.Printf("[Relay] Client %s subscribed to %s", client.SocketID, channelName)
	}
}

func (h *Hub) handleUnsubscribe(client *Client, msg *protocol.Message) {
	var subData protocol.SubscribeData
	if err := json.Unmarshal(msg.Data, &subData); err != nil {
		return
	}

	channelName := subData.Channel
	h.mu.Lock()
	ch, ok := h.channels[channelName]
	h.mu.Unlock()

	if !ok {
		return
	}

	member := ch.Unsubscribe(client)
	delete(client.subscriptions, channelName)

	if ch.Type == protocol.ChannelTypePresence && member != nil {
		go h.broadcastMemberRemoved(ch, member)
	}

	h.mu.Lock()
	if ch.IsEmpty() {
		delete(h.channels, channelName)
	}
	h.mu.Unlock()
}

func (h *Hub) handlePing(client *Client) {
	pong, _ := protocol.NewMessage(protocol.EventPong, "", nil)
	client.SendMessage(pong)
}

func (h *Hub) handleClientEvent(client *Client, msg *protocol.Message) {
	// Client events are forwarded to the channel, excluding the sender
	if msg.Channel == "" {
		return
	}

	h.mu.RLock()
	ch, ok := h.channels[msg.Channel]
	h.mu.RUnlock()

	if !ok {
		return
	}

	// Only subscribed clients can send client events
	if !client.subscriptions[msg.Channel] {
		return
	}

	// Only private and presence channels allow client events
	if ch.Type == protocol.ChannelTypePublic {
		return
	}

	ch.Broadcast(msg, client.SocketID)
}

func (h *Hub) handlePublish(req *protocol.PublishRequest) {
	h.mu.RLock()
	ch, ok := h.channels[req.Channel]
	h.mu.RUnlock()

	if !ok {
		// No subscribers — that's fine, not an error
		return
	}

	msg := &protocol.Message{
		Event:   req.Event,
		Channel: req.Channel,
		Data:    req.Data,
	}

	ch.Broadcast(msg, req.SocketID)

	// Record to event log ring buffer
	h.mu.Lock()
	h.eventLog[h.eventLogPos] = EventLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Channel:   req.Channel,
		Event:     req.Event,
	}
	h.eventLogPos = (h.eventLogPos + 1) % len(h.eventLog)
	if h.eventLogLen < len(h.eventLog) {
		h.eventLogLen++
	}
	h.mu.Unlock()

	if h.cfg.Debug {
		log.Printf("[Relay] Published event=%s channel=%s", req.Event, req.Channel)
	}
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

// --- Auth Validation ---

func (h *Hub) validateAuth(socketID, channelName, authToken, channelData string) bool {
	return auth.Validate(h.cfg.AppKey, h.cfg.AppSecret, socketID, channelName, authToken, channelData)
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

// GetChannels returns info about all active channels.
func (h *Hub) GetChannels() []ChannelInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	infos := make([]ChannelInfo, 0, len(h.channels))
	for _, ch := range h.channels {
		infos = append(infos, ch.Info())
	}
	return infos
}

// GetChannel returns info about a specific channel, or nil if not found.
func (h *Hub) GetChannel(name string) *ChannelInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ch, ok := h.channels[name]
	if !ok {
		return nil
	}
	info := ch.Info()
	return &info
}

// GetChannelMembers returns presence members for a channel.
func (h *Hub) GetChannelMembers(name string) map[string]*protocol.PresenceMember {
	h.mu.RLock()
	ch, ok := h.channels[name]
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

// GetEventLog returns the last n events from the ring buffer in reverse chronological order.
func (h *Hub) GetEventLog(n int) []EventLogEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if n > h.eventLogLen {
		n = h.eventLogLen
	}
	result := make([]EventLogEntry, n)
	for i := 0; i < n; i++ {
		idx := (h.eventLogPos - 1 - i + len(h.eventLog)) % len(h.eventLog)
		result[i] = h.eventLog[idx]
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

	// Give clients 1 second to receive the shutdown message
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
