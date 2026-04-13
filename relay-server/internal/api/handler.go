package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/relayhq/relay-server/internal/apps"
	"github.com/relayhq/relay-server/internal/auth"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/history"
	"github.com/relayhq/relay-server/internal/hub"
	"github.com/relayhq/relay-server/internal/protocol"
	"github.com/relayhq/relay-server/internal/ratelimit"
)

// Handler holds the REST API route handlers.
type Handler struct {
	hub            *hub.Hub
	cfg            *config.Config
	registry       *apps.AppRegistry
	publishLimiter *ratelimit.Limiter
}

// NewHandler creates an API handler.
func NewHandler(h *hub.Hub, cfg *config.Config, registry *apps.AppRegistry) *Handler {
	return &Handler{
		hub:            h,
		cfg:            cfg,
		registry:       registry,
		publishLimiter: ratelimit.NewLimiter(1000, 1*time.Minute),
	}
}

// RateLimitMiddleware checks the publish rate limit per IP.
func (h *Handler) RateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !h.publishLimiter.Allow(ip) {
			jsonError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	}
}

// AuthenticateMiddleware looks up the app by ID from the URL and checks Bearer {appSecret}.
func (h *Handler) AuthenticateMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		appID := mux.Vars(r)["appId"]
		app, ok := h.registry.LookupByID(appID)
		if !ok {
			jsonError(w, http.StatusUnauthorized, "Unknown app ID")
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != app.Secret {
			jsonError(w, http.StatusForbidden, "Invalid app secret")
			return
		}
		next(w, r)
	}
}

// PublishEvent handles POST /apps/{appId}/events
func (h *Handler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	var req protocol.PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if req.Channel == "" || req.Event == "" {
		jsonError(w, http.StatusUnprocessableEntity, "channel and event are required")
		return
	}

	req.AppID = appID
	h.hub.Publish <- &req
	jsonOK(w, map[string]any{"ok": true})
}

// PublishBatch handles POST /apps/{appId}/events/batch
func (h *Handler) PublishBatch(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	var req protocol.BatchPublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	for i := range req.Batch {
		req.Batch[i].AppID = appID
		h.hub.Publish <- &req.Batch[i]
	}
	jsonOK(w, map[string]any{"ok": true, "count": len(req.Batch)})
}

// GetChannels handles GET /apps/{appId}/channels
// Returns a map of channel names to channel info, suitable for the Channel Inspector UI.
func (h *Handler) GetChannels(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	channels := h.hub.GetChannels(appID)

	channelMap := make(map[string]any, len(channels))
	for _, ch := range channels {
		entry := map[string]any{
			"type":               ch.Type,
			"subscription_count": ch.SubscriberCount,
		}
		if ch.Type == "presence" {
			entry["user_count"] = ch.UserCount
		}
		channelMap[ch.Name] = entry
	}
	jsonOK(w, map[string]any{"channels": channelMap})
}

// GetAllChannels handles GET /dashboard/api/channels (no auth, all apps).
func (h *Handler) GetAllChannels(w http.ResponseWriter, r *http.Request) {
	channels := h.hub.GetAllChannels()
	jsonOK(w, map[string]any{"channels": channels})
}

// GetChannel handles GET /apps/{appId}/channels/{channelName}
func (h *Handler) GetChannel(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	name := mux.Vars(r)["channelName"]
	info := h.hub.GetChannel(appID, name)
	if info == nil {
		jsonError(w, http.StatusNotFound, "Channel not found")
		return
	}
	jsonOK(w, info)
}

// GetChannelUsers handles GET /apps/{appId}/channels/{channelName}/users
func (h *Handler) GetChannelUsers(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	name := mux.Vars(r)["channelName"]
	members := h.hub.GetChannelMembers(appID, name)
	if members == nil {
		jsonError(w, http.StatusNotFound, "Channel not found")
		return
	}

	users := make([]any, 0, len(members))
	for _, m := range members {
		users = append(users, m)
	}
	jsonOK(w, map[string]any{"users": users})
}

// AuthChannel handles POST /apps/{appId}/auth
func (h *Handler) AuthChannel(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	app, ok := h.registry.LookupByID(appID)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unknown app ID")
		return
	}

	var req struct {
		SocketID    string `json:"socket_id"`
		ChannelName string `json:"channel_name"`
		ChannelData string `json:"channel_data,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if req.SocketID == "" || req.ChannelName == "" {
		jsonError(w, http.StatusUnprocessableEntity, "socket_id and channel_name are required")
		return
	}

	token := auth.Sign(app.Key, app.Secret, req.SocketID, req.ChannelName, req.ChannelData)

	resp := map[string]string{"auth": token}
	if req.ChannelData != "" {
		resp["channel_data"] = req.ChannelData
	}
	jsonOK(w, resp)
}

// GetChannelEvents handles GET /apps/{appId}/channels/{channelName}/events
// Supports cursor-based pagination via ?limit=N&cursor=OPAQUE_CURSOR
func (h *Handler) GetChannelEvents(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	channelName := mux.Vars(r)["channelName"]

	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}

	if h.hub.History == nil {
		jsonOK(w, map[string]any{"events": []any{}, "next_cursor": nil})
		return
	}

	var events []history.Event
	cursor := r.URL.Query().Get("cursor")
	if cursor != "" {
		beforeID := history.DecodeCursor(cursor)
		if beforeID > 0 {
			events = h.hub.History.GetBeforeID(appID, channelName, beforeID, limit)
		}
	}
	if events == nil {
		events = h.hub.History.GetNewest(appID, channelName, limit)
	}
	if events == nil {
		events = make([]history.Event, 0)
	}

	// Build next_cursor from the oldest event in the result set
	var nextCursor any
	if len(events) == limit && len(events) > 0 {
		oldestID := events[len(events)-1].ID
		nextCursor = history.EncodeCursor(oldestID)
	}

	jsonOK(w, map[string]any{"events": events, "next_cursor": nextCursor})
}

// GetEventLog handles GET /apps/{appId}/events/log
func (h *Handler) GetEventLog(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	events := h.hub.GetEventLog(20, appID)
	jsonOK(w, map[string]any{"events": events})
}

// GetAllEvents handles GET /dashboard/api/events (no auth, all apps).
func (h *Handler) GetAllEvents(w http.ResponseWriter, r *http.Request) {
	events := h.hub.GetEventLog(20, "")
	jsonOK(w, map[string]any{"events": events})
}

// GetAppStats handles GET /apps/{appId}/stats
// Returns per-app connection count, peak connections, and message count.
func (h *Handler) GetAppStats(w http.ResponseWriter, r *http.Request) {
	appID := mux.Vars(r)["appId"]
	jsonOK(w, map[string]any{
		"connections":      h.hub.AppConnCount(appID),
		"peak_connections": h.hub.AppPeakConnCount(appID),
		"messages_count":   h.hub.AppMsgCount(appID),
	})
}

// GetStats handles GET /stats (global, no auth)
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{
		"connections": h.hub.ConnectionCount(),
		"channels":    h.hub.ChannelCount(),
		"apps":        h.hub.AppStats(),
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

func extractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
