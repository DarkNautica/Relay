package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/hub"
	"github.com/relayhq/relay-server/internal/protocol"
)

// Handler holds the REST API route handlers.
type Handler struct {
	hub *hub.Hub
	cfg *config.Config
}

// NewHandler creates an API handler.
func NewHandler(h *hub.Hub, cfg *config.Config) *Handler {
	return &Handler{hub: h, cfg: cfg}
}

// AuthenticateMiddleware returns a wrapper that checks Bearer {appSecret}.
func (h *Handler) AuthenticateMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			jsonError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != h.cfg.AppSecret {
			jsonError(w, http.StatusForbidden, "Invalid app secret")
			return
		}
		next(w, r)
	}
}

// PublishEvent handles POST /apps/{appId}/events
func (h *Handler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	var req protocol.PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if req.Channel == "" || req.Event == "" {
		jsonError(w, http.StatusUnprocessableEntity, "channel and event are required")
		return
	}

	h.hub.Publish <- &req
	jsonOK(w, map[string]any{"ok": true})
}

// PublishBatch handles POST /apps/{appId}/events/batch
func (h *Handler) PublishBatch(w http.ResponseWriter, r *http.Request) {
	var req protocol.BatchPublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	for i := range req.Batch {
		h.hub.Publish <- &req.Batch[i]
	}
	jsonOK(w, map[string]any{"ok": true, "count": len(req.Batch)})
}

// GetChannels handles GET /apps/{appId}/channels
func (h *Handler) GetChannels(w http.ResponseWriter, r *http.Request) {
	channels := h.hub.GetChannels()
	jsonOK(w, map[string]any{"channels": channels})
}

// GetChannel handles GET /apps/{appId}/channels/{channelName}
func (h *Handler) GetChannel(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["channelName"]
	info := h.hub.GetChannel(name)
	if info == nil {
		jsonError(w, http.StatusNotFound, "Channel not found")
		return
	}
	jsonOK(w, info)
}

// GetChannelUsers handles GET /apps/{appId}/channels/{channelName}/users
func (h *Handler) GetChannelUsers(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["channelName"]
	members := h.hub.GetChannelMembers(name)
	if members == nil {
		jsonError(w, http.StatusNotFound, "Channel not found")
		return
	}

	// Flatten to a list of user objects
	users := make([]any, 0, len(members))
	for _, m := range members {
		users = append(users, m)
	}
	jsonOK(w, map[string]any{"users": users})
}

// GetStats handles GET /stats
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{
		"connections": h.hub.ConnectionCount(),
		"channels":    h.hub.ChannelCount(),
	})
}

// --- JSON helpers ---

func jsonOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
