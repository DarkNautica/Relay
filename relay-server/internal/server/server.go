package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/relayhq/relay-server/internal/api"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/dashboard"
	"github.com/relayhq/relay-server/internal/hub"
	wshandler "github.com/relayhq/relay-server/internal/websocket"
)

// Server wraps the HTTP server and all its dependencies.
type Server struct {
	cfg        *config.Config
	hub        *hub.Hub
	httpServer *http.Server
}

// New creates a new Server.
func New(cfg *config.Config, h *hub.Hub) *Server {
	s := &Server{
		cfg: cfg,
		hub: h,
	}
	s.httpServer = &http.Server{
		Addr:         cfg.Addr(),
		Handler:      s.buildRouter(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

// Start begins serving HTTP requests. Blocks until the server closes.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.httpServer.Shutdown(ctx)
}

// buildRouter wires up all routes.
func (s *Server) buildRouter() http.Handler {
	r := mux.NewRouter()

	// WebSocket endpoint — this is where browsers connect
	wsHandler := wshandler.NewHandler(s.hub, s.cfg)
	r.Handle("/app/{appKey}", wsHandler)

	// REST API — authenticated with Bearer {appSecret}
	apiHandler := api.NewHandler(s.hub, s.cfg)
	auth := apiHandler.AuthenticateMiddleware
	rl := apiHandler.RateLimitMiddleware

	appsRouter := r.PathPrefix("/apps/{appId}").Subrouter()
	appsRouter.HandleFunc("/events", rl(auth(apiHandler.PublishEvent))).Methods(http.MethodPost)
	appsRouter.HandleFunc("/events/batch", rl(auth(apiHandler.PublishBatch))).Methods(http.MethodPost)
	appsRouter.HandleFunc("/channels", auth(apiHandler.GetChannels)).Methods(http.MethodGet)
	appsRouter.HandleFunc("/channels/{channelName}", auth(apiHandler.GetChannel)).Methods(http.MethodGet)
	appsRouter.HandleFunc("/channels/{channelName}/users", auth(apiHandler.GetChannelUsers)).Methods(http.MethodGet)

	// Auth endpoint — no Bearer auth (the user's app authenticates this)
	appsRouter.HandleFunc("/auth", apiHandler.AuthChannel).Methods(http.MethodPost)

	// Event log for dashboard
	appsRouter.HandleFunc("/events/log", auth(apiHandler.GetEventLog)).Methods(http.MethodGet)

	// Stats endpoint (no auth — useful for health checks)
	r.HandleFunc("/stats", apiHandler.GetStats).Methods(http.MethodGet)

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}).Methods(http.MethodGet)

	// Dashboard
	if s.cfg.DashboardEnabled {
		dash := dashboard.NewHandler()
		r.Handle(s.cfg.DashboardPath, dash).Methods(http.MethodGet)
	}

	// CORS middleware
	return corsMiddleware(r)
}

// corsMiddleware adds permissive CORS headers.
// In production, restrict the allowed origins.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
