package protocol

import "encoding/json"

// ChannelType represents the type of a Relay channel.
type ChannelType int

const (
	ChannelTypePublic   ChannelType = iota // No auth required
	ChannelTypePrivate                     // Requires server auth
	ChannelTypePresence                    // Auth + member tracking
)

// String returns a human-readable channel type name.
func (ct ChannelType) String() string {
	switch ct {
	case ChannelTypePrivate:
		return "private"
	case ChannelTypePresence:
		return "presence"
	default:
		return "public"
	}
}

// ChannelTypeFromName infers the channel type from its name prefix.
func ChannelTypeFromName(name string) ChannelType {
	if len(name) >= 8 && name[:8] == "private-" {
		return ChannelTypePrivate
	}
	if len(name) >= 9 && name[:9] == "presence-" {
		return ChannelTypePresence
	}
	return ChannelTypePublic
}

// --- Relay System Events ---

const (
	// Server → Client
	EventConnectionEstablished = "relay:connection_established"
	EventSubscriptionSucceeded = "relay:subscription_succeeded"
	EventSubscriptionError     = "relay:subscription_error"
	EventMemberAdded           = "relay:member_added"
	EventMemberRemoved         = "relay:member_removed"
	EventError                 = "relay:error"
	EventPong                  = "relay:pong"

	// Client → Server
	EventSubscribe   = "relay:subscribe"
	EventUnsubscribe = "relay:unsubscribe"
	EventPing        = "relay:ping"

	// Pusher protocol aliases (for compatibility)
	PusherEventConnectionEstablished = "pusher:connection_established"
	PusherEventSubscriptionSucceeded = "pusher_internal:subscription_succeeded"
	PusherEventSubscribe             = "pusher:subscribe"
	PusherEventUnsubscribe           = "pusher:unsubscribe"
	PusherEventPing                  = "pusher:ping"
	PusherEventPong                  = "pusher:pong"
	PusherEventError                 = "pusher:error"
	PusherEventMemberAdded           = "pusher_internal:member_added"
	PusherEventMemberRemoved         = "pusher_internal:member_removed"
)

// Message is the core wire format for all WebSocket messages.
type Message struct {
	Event   string          `json:"event"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// NewMessage creates a Message with a string data payload.
func NewMessage(event, channel string, data any) (*Message, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &Message{
		Event:   event,
		Channel: channel,
		Data:    raw,
	}, nil
}

// Encode serialises a Message to JSON bytes ready to send over the wire.
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// --- Specific Data Payloads ---

// ConnectionData is sent inside relay:connection_established.
type ConnectionData struct {
	SocketID        string `json:"socket_id"`
	ActivityTimeout int    `json:"activity_timeout"`
}

// SubscribeData is the data payload for relay:subscribe from the client.
type SubscribeData struct {
	Channel     string `json:"channel"`
	Auth        string `json:"auth,omitempty"`
	ChannelData string `json:"channel_data,omitempty"`
}

// PresenceMember represents a member in a presence channel.
type PresenceMember struct {
	ID   any `json:"id"`
	Info any `json:"user_info,omitempty"`
}

// PresenceData is sent inside relay:subscription_succeeded for presence channels.
type PresenceData struct {
	Presence struct {
		Count int                        `json:"count"`
		IDs   []any                      `json:"ids"`
		Hash  map[string]*PresenceMember `json:"hash"`
	} `json:"presence"`
}

// ErrorData is the payload for relay:error.
type ErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PublishRequest is the body for the HTTP publish API.
type PublishRequest struct {
	Channel  string          `json:"channel"`
	Event    string          `json:"event"`
	Data     json.RawMessage `json:"data"`
	SocketID string          `json:"socket_id,omitempty"` // Exclude this socket from receiving
}

// BatchPublishRequest is the body for the HTTP batch publish API.
type BatchPublishRequest struct {
	Batch []PublishRequest `json:"batch"`
}
