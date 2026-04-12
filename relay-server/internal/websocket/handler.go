package websocket

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/relayhq/relay-server/internal/apps"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/hub"
	"github.com/relayhq/relay-server/internal/ratelimit"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Handler implements http.Handler for the WebSocket upgrade endpoint.
type Handler struct {
	hub      *hub.Hub
	cfg      *config.Config
	registry *apps.AppRegistry
	limiter  *ratelimit.Limiter
}

// NewHandler creates a WebSocket handler.
func NewHandler(h *hub.Hub, cfg *config.Config, registry *apps.AppRegistry) http.Handler {
	return &Handler{
		hub:      h,
		cfg:      cfg,
		registry: registry,
		limiter:  ratelimit.NewLimiter(10, 1*time.Minute),
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

	// Look up app by key from the URL
	appKey := mux.Vars(r)["appKey"]
	app, ok := h.registry.Lookup(appKey)
	if !ok {
		// Upgrade then reject with close code 4001 (Pusher-compatible)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4001, "Invalid app key"))
		conn.Close()
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Relay] WebSocket upgrade failed: %v", err)
		return
	}

	client := hub.NewClientConn(h.hub, conn, h.cfg, app)
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
