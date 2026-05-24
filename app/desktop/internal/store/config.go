package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const (
	DefaultGatewayURL = "http://localhost:8888"
	DefaultWSURL      = "ws://localhost:8888/ws"
)

type UserProfile struct {
	UserID   int64  `json:"user_id"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}
type Config struct {
	GatewayURL   string      `json:"gateway_url"`
	WSURL        string      `json:"ws_url"`
	DeviceID     string      `json:"device_id"`
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresAt    int64       `json:"expires_at"`
	User         UserProfile `json:"user"`
}

type ConfigStore struct{ path string }

func NewConfigStore(dir string) *ConfigStore {
	return &ConfigStore{path: filepath.Join(dir, "config.json")}
}
func DefaultConfig() Config {
	return Config{GatewayURL: DefaultGatewayURL, WSURL: DefaultWSURL, DeviceID: uuid.NewString()}
}
func (s *ConfigStore) Load() (Config, error) {
	cfg := DefaultConfig()
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = DefaultGatewayURL
	}
	if cfg.WSURL == "" {
		cfg.WSURL = DefaultWSURL
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = uuid.NewString()
	}
	return cfg, nil
}
func (s *ConfigStore) Save(cfg Config) error {
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = DefaultGatewayURL
	}
	if cfg.WSURL == "" {
		cfg.WSURL = DefaultWSURL
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = uuid.NewString()
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}
