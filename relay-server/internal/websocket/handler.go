package websocket

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/hub"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins — tighten in production
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler implements http.Handler for the WebSocket upgrade endpoint.
type Handler struct {
	hub *hub.Hub
	cfg *config.Config
}

// NewHandler creates a WebSocket handler.
func NewHandler(h *hub.Hub, cfg *config.Config) http.Handler {
	return &Handler{hub: h, cfg: cfg}
}

// ServeHTTP upgrades the HTTP connection to WebSocket, creates a hub client,
// and starts the read/write pumps.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Relay] WebSocket upgrade failed: %v", err)
		return
	}

	client := hub.NewClientConn(h.hub, conn, h.cfg)
	h.hub.RegisterClient(client)

	go hub.StartWritePump(client)
	go hub.StartReadPump(client)
}
