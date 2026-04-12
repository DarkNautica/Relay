package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the Relay server.
// All values are loaded from environment variables, with sensible defaults.
type Config struct {
	// Server
	Host string
	Port int

	// Application credentials
	AppID     string
	AppKey    string
	AppSecret string

	// Limits
	MaxConnections        int
	MaxChannelNameLength  int
	MaxEventPayloadKB     int

	// Timeouts (seconds)
	PingInterval int
	PingTimeout  int

	// Dashboard
	DashboardEnabled bool
	DashboardPath    string

	// Debug
	Debug bool
}

// Load reads configuration from the environment.
// It also attempts to load a .env file from the current directory.
func Load() *Config {
	// Attempt to load .env — not an error if it doesn't exist
	if err := godotenv.Load(); err != nil {
		log.Println("[Relay] No .env file found, using environment variables")
	}

	return &Config{
		Host:      getEnv("RELAY_HOST", "0.0.0.0"),
		Port:      getEnvInt("RELAY_PORT", 6001),
		AppID:     getEnv("RELAY_APP_ID", "relay-app"),
		AppKey:    getEnv("RELAY_APP_KEY", "relay-key"),
		AppSecret: getEnv("RELAY_APP_SECRET", "relay-secret-change-me"),

		MaxConnections:       getEnvInt("RELAY_MAX_CONNECTIONS", 10000),
		MaxChannelNameLength: getEnvInt("RELAY_MAX_CHANNEL_NAME_LENGTH", 200),
		MaxEventPayloadKB:    getEnvInt("RELAY_MAX_EVENT_PAYLOAD_KB", 100),

		PingInterval: getEnvInt("RELAY_PING_INTERVAL", 120),
		PingTimeout:  getEnvInt("RELAY_PING_TIMEOUT", 30),

		DashboardEnabled: getEnvBool("RELAY_DASHBOARD_ENABLED", true),
		DashboardPath:    getEnv("RELAY_DASHBOARD_PATH", "/dashboard"),

		Debug: getEnvBool("RELAY_DEBUG", false),
	}
}

// Addr returns the full host:port address string.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Validate returns an error if the config is missing critical values.
func (c *Config) Validate() error {
	if c.AppKey == "" {
		return fmt.Errorf("RELAY_APP_KEY is required")
	}
	if c.AppSecret == "" {
		return fmt.Errorf("RELAY_APP_SECRET is required")
	}
	if c.AppSecret == "relay-secret-change-me" {
		log.Println("[Relay] WARNING: Using default app secret. Set RELAY_APP_SECRET in production.")
	}
	return nil
}

// --- helpers ---

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
