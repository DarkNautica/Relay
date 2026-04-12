package hub

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/relayhq/relay-server/internal/protocol"
)

// Channel represents a Relay channel.
// Channels are the rooms that clients subscribe to and events are broadcast through.
type Channel struct {
	mu sync.RWMutex

	// The channel name (e.g. "chat", "private-user.1", "presence-room.42")
	Name string

	// The channel type inferred from the name
	Type protocol.ChannelType

	// All clients currently subscribed to this channel
	clients map[*Client]bool

	// Presence members — only populated for presence channels
	// Key is socket ID, value is member info
	members map[string]*protocol.PresenceMember
}

// newChannel creates a new Channel with the correct type inferred from its name.
func newChannel(name string) *Channel {
	return &Channel{
		Name:    name,
		Type:    protocol.ChannelTypeFromName(name),
		clients: make(map[*Client]bool),
		members: make(map[string]*protocol.PresenceMember),
	}
}

// Subscribe adds a client to the channel.
// For presence channels, memberJSON should be the raw channel_data payload.
func (ch *Channel) Subscribe(client *Client, memberJSON string) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.clients[client] = true

	// For presence channels, parse and store member info
	if ch.Type == protocol.ChannelTypePresence && memberJSON != "" {
		var member protocol.PresenceMember
		if err := json.Unmarshal([]byte(memberJSON), &member); err != nil {
			return fmt.Errorf("invalid channel_data for presence channel: %w", err)
		}
		ch.members[client.SocketID] = &member
	}

	return nil
}

// Unsubscribe removes a client from the channel.
// Returns the member data if it was a presence channel (for broadcasting member_removed).
func (ch *Channel) Unsubscribe(client *Client) *protocol.PresenceMember {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	delete(ch.clients, client)

	// Remove and return presence member if applicable
	if ch.Type == protocol.ChannelTypePresence {
		member := ch.members[client.SocketID]
		delete(ch.members, client.SocketID)
		return member
	}

	return nil
}

// Broadcast sends a message to all subscribed clients.
// If excludeSocketID is set, that client will not receive the message.
func (ch *Channel) Broadcast(msg *protocol.Message, excludeSocketID string) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	data, err := msg.Encode()
	if err != nil {
		return
	}

	for client := range ch.clients {
		if excludeSocketID != "" && client.SocketID == excludeSocketID {
			continue
		}
		client.SendRaw(data)
	}
}

// ClientCount returns the number of subscribed clients.
func (ch *Channel) ClientCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.clients)
}

// IsEmpty returns true if no clients are subscribed.
func (ch *Channel) IsEmpty() bool {
	return ch.ClientCount() == 0
}

// PresenceData builds the presence payload for subscription_succeeded.
func (ch *Channel) PresenceData() *protocol.PresenceData {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	data := &protocol.PresenceData{}
	data.Presence.Hash = make(map[string]*protocol.PresenceMember)

	for socketID, member := range ch.members {
		data.Presence.IDs = append(data.Presence.IDs, member.ID)
		data.Presence.Hash[fmt.Sprintf("%v", socketID)] = member
	}
	data.Presence.Count = len(ch.members)

	return data
}

// GetMembers returns a snapshot of all presence members.
func (ch *Channel) GetMembers() map[string]*protocol.PresenceMember {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	snapshot := make(map[string]*protocol.PresenceMember, len(ch.members))
	for k, v := range ch.members {
		snapshot[k] = v
	}
	return snapshot
}

// Info returns a summary of the channel for the API.
type ChannelInfo struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	SubscriberCount int    `json:"subscriber_count"`
	Occupied        bool   `json:"occupied"`
	AppID           string `json:"app_id,omitempty"`
}

func (ch *Channel) Info() ChannelInfo {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ChannelInfo{
		Name:            ch.Name,
		Type:            ch.Type.String(),
		SubscriberCount: len(ch.clients),
		Occupied:        len(ch.clients) > 0,
	}
}
