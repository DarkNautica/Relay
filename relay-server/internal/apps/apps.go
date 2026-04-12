package apps

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/relayhq/relay-server/internal/config"
)

// WebhookConfig defines a webhook endpoint and the events it subscribes to.
type WebhookConfig struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// App represents a single application registered with the Relay server.
type App struct {
	ID             string          `json:"id"`
	Key            string          `json:"key"`
	Secret         string          `json:"secret"`
	MaxConnections int             `json:"max_connections"`
	History        bool            `json:"history"`
	HistoryLimit   int             `json:"history_limit"`
	Webhooks       []WebhookConfig `json:"webhooks"`
}

// AppRegistry holds all registered apps and provides fast lookups by key or ID.
type AppRegistry struct {
	mu    sync.RWMutex
	apps  []*App
	byID  map[string]*App
	byKey map[string]*App
}

// NewRegistry creates an empty AppRegistry.
func NewRegistry() *AppRegistry {
	return &AppRegistry{
		byID:  make(map[string]*App),
		byKey: make(map[string]*App),
	}
}

// Register adds an app to the registry.
func (r *AppRegistry) Register(app *App) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if app.HistoryLimit == 0 {
		app.HistoryLimit = 100
	}
	r.apps = append(r.apps, app)
	r.byID[app.ID] = app
	r.byKey[app.Key] = app
}

// Lookup finds an app by its public key.
func (r *AppRegistry) Lookup(key string) (*App, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	app, ok := r.byKey[key]
	return app, ok
}

// LookupByID finds an app by its ID.
func (r *AppRegistry) LookupByID(id string) (*App, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	app, ok := r.byID[id]
	return app, ok
}

// All returns a copy of all registered apps.
func (r *AppRegistry) All() []*App {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*App, len(r.apps))
	copy(out, r.apps)
	return out
}

// LoadFromFile reads apps from a JSON file. Returns nil, err if the file
// doesn't exist or can't be parsed.
func LoadFromFile(path string) (*AppRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var apps []App
	if err := json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}
	reg := NewRegistry()
	for i := range apps {
		reg.Register(&apps[i])
	}
	return reg, nil
}

// LoadFromConfig creates a registry with a single app from environment config.
func LoadFromConfig(cfg *config.Config) *AppRegistry {
	reg := NewRegistry()
	reg.Register(&App{
		ID:             cfg.AppID,
		Key:            cfg.AppKey,
		Secret:         cfg.AppSecret,
		MaxConnections: cfg.MaxConnections,
	})
	return reg
}
