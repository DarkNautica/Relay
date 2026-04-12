package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/relayhq/relay-server/internal/apps"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/history"
	"github.com/relayhq/relay-server/internal/hub"
	"github.com/relayhq/relay-server/internal/server"
	"github.com/relayhq/relay-server/internal/webhook"
)

func main() {
	cfg := config.Load()

	if cfg.Debug {
		log.Printf("[Relay] Debug mode enabled")
	}

	// Load app registry: try apps.json first, fall back to .env config
	registry, err := apps.LoadFromFile("apps.json")
	if err != nil {
		if cfg.Debug {
			log.Printf("[Relay] No apps.json found, using single app from environment")
		}
		registry = apps.LoadFromConfig(cfg)
	} else {
		log.Printf("[Relay] Loaded %d app(s) from apps.json", len(registry.All()))
	}

	h := hub.NewHub(cfg, registry)
	h.History = history.NewStore(100)
	h.Webhooks = webhook.NewDispatcher()
	go h.Run()

	srv := server.New(cfg, h, registry)

	go func() {
		log.Printf("[Relay] Server starting on %s", cfg.Addr())
		for _, app := range registry.All() {
			log.Printf("[Relay] App: id=%s key=%s", app.ID, app.Key)
		}
		if cfg.DashboardEnabled {
			log.Printf("[Relay] Dashboard: http://%s%s", cfg.Addr(), cfg.DashboardPath)
		}

		if err := srv.Start(); err != nil {
			log.Fatalf("[Relay] Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Relay] Shutting down gracefully...")
	h.Shutdown()
	srv.Shutdown()
	log.Println("[Relay] Goodbye.")
}
