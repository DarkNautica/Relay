package websocket

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/hub"
	"github.com/relayhq/relay-server/internal/ratelimit"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins — tighten in production
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler implements http.Handler for the WebSocket upgrade endpoint.
type Handler struct {
	hub     *hub.Hub
	cfg     *config.Config
	limiter *ratelimit.Limiter
}

// NewHandler creates a WebSocket handler.
func NewHandler(h *hub.Hub, cfg *config.Config) http.Handler {
	return &Handler{
		hub:     h,
		cfg:     cfg,
		limiter: ratelimit.NewLimiter(10, 1*time.Minute),
	}
}

// ServeHTTP upgrades the HTTP connection to WebSocket, creates a hub client,
// and starts the read/write pumps.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := extractIP(r)
	if !h.limiter.Allow(ip) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
		return
	}

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

// extractIP returns the client IP from the request, stripping the port.
func extractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
