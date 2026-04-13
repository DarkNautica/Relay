package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/relayhq/relay-server/internal/apps"
	"github.com/relayhq/relay-server/internal/config"
	"github.com/relayhq/relay-server/internal/eventstore"
	"github.com/relayhq/relay-server/internal/history"
	"github.com/relayhq/relay-server/internal/hub"
	"github.com/relayhq/relay-server/internal/server"
	"github.com/relayhq/relay-server/internal/webhook"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	// Structured JSON logging for all server output
	logLevel := slog.LevelInfo
	cfg := config.Load()
	if cfg.Debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	slog.Info("starting relay server", "version", version)

	if cfg.Debug {
		slog.Debug("debug mode enabled")
	}

	// Load app registry: try apps.json first, fall back to .env config
	registry, err := apps.LoadFromFile("apps.json")
	if err != nil {
		if cfg.Debug {
			slog.Debug("no apps.json found, using single app from environment")
		}
		registry = apps.LoadFromConfig(cfg)
	} else {
		slog.Info("loaded apps from apps.json", "count", len(registry.All()))
	}

	h := hub.NewHub(cfg, registry)
	h.History = history.NewStore(100)
	h.Webhooks = webhook.NewDispatcher()
	h.EventStore = eventstore.NewStore(1000)
	go h.Run()

	srv := server.New(cfg, h, registry)

	go func() {
		slog.Info("server listening", "addr", cfg.Addr())
		for _, app := range registry.All() {
			slog.Info("app registered", "app_id", app.ID, "app_key", app.Key,
				"max_connections", app.MaxConnections)
		}
		if cfg.DashboardEnabled {
			slog.Info("dashboard enabled",
				"url", fmt.Sprintf("http://%s%s", cfg.Addr(), cfg.DashboardPath))
		}

		if err := srv.Start(); err != nil {
			slog.Error("server error", "error", err.Error())
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gracefully")
	h.Shutdown()
	srv.Shutdown()
	slog.Info("shutdown complete")
}
