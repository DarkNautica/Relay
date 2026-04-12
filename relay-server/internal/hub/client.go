package hub

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/gorilla/websocket"
	"github.com/relayhq/relay-server/internal/apps"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/protocol"
)

const (
	// Maximum message size allowed from a client (bytes)
	maxMessageSize = 102400 // 100KB

	// Time allowed to write a message to the client
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client
	pongWait = 60 * time.Second

	// Send pings at this interval (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Size of the outbound message buffer per client
	sendBufferSize = 256
)

// Client represents a single connected WebSocket client.
// Each client runs two goroutines: readPump and writePump.
type Client struct {
	// Unique identifier for this connection
	SocketID string

	// The hub this client belongs to
	hub *Hub

	// The underlying WebSocket connection
	conn *websocket.Conn

	// Buffered channel of outbound messages
	send chan []byte

	// Set of channel names this client is subscribed to
	subscriptions map[string]bool

	// Config reference
	cfg *config.Config

	// The app this client authenticated with
	app *apps.App
}

// newClient creates a new Client with a generated socket ID.
func newClient(hub *Hub, conn *websocket.Conn, cfg *config.Config, app *apps.App) *Client {
	return &Client{
		SocketID:      generateSocketID(),
		hub:           hub,
		conn:          conn,
		send:          make(chan []byte, sendBufferSize),
		subscriptions: make(map[string]bool),
		cfg:           cfg,
		app:           app,
	}
}

// generateSocketID creates a Pusher-compatible socket ID.
// Format: {random}.{random}
func generateSocketID() string {
	return fmt.Sprintf("%d.%d", rand.Int63n(999999999), rand.Int63n(999999999))
}

// SendMessage encodes a protocol.Message and queues it for sending.
func (c *Client) SendMessage(msg *protocol.Message) error {
	data, err := msg.Encode()
	if err != nil {
		return err
	}
	select {
	case c.send <- data:
		return nil
	default:
		// Buffer is full — client is too slow, disconnect
		return fmt.Errorf("client %s send buffer full", c.SocketID)
	}
}

// SendRaw queues raw bytes for sending to this client.
func (c *Client) SendRaw(data []byte) {
	select {
	case c.send <- data:
	default:
		log.Printf("[Relay] Client %s send buffer full, dropping message", c.SocketID)
	}
}

// readPump pumps messages from the WebSocket connection to the hub.
// This runs in a dedicated goroutine per client.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, rawMessage, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Relay] Client %s unexpected close: %v", c.SocketID, err)
			}
			break
		}

		// Parse the incoming message
		var msg protocol.Message
		if err := json.Unmarshal(rawMessage, &msg); err != nil {
			log.Printf("[Relay] Client %s invalid message: %v", c.SocketID, err)
			continue
		}

		// Route message to the hub for processing
		c.hub.incoming <- &incomingMessage{
			client:  c,
			message: &msg,
		}
	}
}

// writePump pumps messages from the send channel to the WebSocket connection.
// This runs in a dedicated goroutine per client.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel — send a close message
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			// Send a WebSocket ping frame to keep the connection alive
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sendConnectionEstablished sends the initial connection event to a new client.
func (c *Client) sendConnectionEstablished() error {
	data := protocol.ConnectionData{
		SocketID:        c.SocketID,
		ActivityTimeout: c.cfg.PingInterval,
	}

	msg, err := protocol.NewMessage(protocol.EventConnectionEstablished, "", data)
	if err != nil {
		return err
	}

	return c.SendMessage(msg)
}
