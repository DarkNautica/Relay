package hub

import (
	"github.com/gorilla/websocket"
	"github.com/relayhq/relay-server/internal/apps"
	"github.com/relayhq/relay-server/internal/config"
)

// NewClientConn creates a new Client from a raw WebSocket connection.
// This is the exported entry point used by the websocket handler package.
func NewClientConn(h *Hub, conn *websocket.Conn, cfg *config.Config, app *apps.App) *Client {
	return newClient(h, conn, cfg, app)
}

// StartReadPump starts the client's read pump in the current goroutine.
// Call this from a dedicated goroutine — it blocks until the connection closes.
func StartReadPump(c *Client) {
	c.readPump()
}

// StartWritePump starts the client's write pump in the current goroutine.
// Call this from a dedicated goroutine — it blocks until the connection closes.
func StartWritePump(c *Client) {
	c.writePump()
}
