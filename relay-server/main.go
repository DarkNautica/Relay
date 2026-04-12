package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/hub"
	"github.com/relayhq/relay-server/internal/server"
)

func main() {
	// Load configuration from environment / .env file
	cfg := config.Load()

	if cfg.Debug {
		log.Printf("[Relay] Debug mode enabled")
	}

	// Create the central hub — this manages all connections and channels
	h := hub.NewHub(cfg)

	// Start the hub in its own goroutine
	// The hub runs an event loop processing connections, disconnections, and messages
	go h.Run()

	// Create and start the HTTP server (WebSocket + REST API)
	srv := server.New(cfg, h)

	// Run server in its own goroutine so we can listen for shutdown signals
	go func() {
		log.Printf("[Relay] Server starting on %s", cfg.Addr())
		log.Printf("[Relay] App Key: %s", cfg.AppKey)
		if cfg.DashboardEnabled {
			log.Printf("[Relay] Dashboard: http://%s%s", cfg.Addr(), cfg.DashboardPath)
		}

		if err := srv.Start(); err != nil {
			log.Fatalf("[Relay] Server error: %v", err)
		}
	}()

	// Block until we receive a shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Relay] Shutting down gracefully...")
	srv.Shutdown()
	log.Println("[Relay] Goodbye.")
}
