package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

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

type AccountProfile struct {
	DeviceID     string      `json:"device_id"`
	AccessToken  string      `json:"access_token,omitempty"`
	RefreshToken string      `json:"refresh_token,omitempty"`
	ExpiresAt    int64       `json:"expires_at,omitempty"`
	User         UserProfile `json:"user"`
	UpdatedAt    int64       `json:"updated_at"`
}

type Config struct {
	GatewayURL   string           `json:"gateway_url"`
	WSURL        string           `json:"ws_url"`
	ActiveUserID int64            `json:"active_user_id,omitempty"`
	Accounts     []AccountProfile `json:"accounts,omitempty"`

	// LegacyCacheUserID is set only in memory while loading a pre-multi-account
	// config. Desktop uses it to migrate cache.db to the matching account once.
	LegacyCacheUserID int64 `json:"-"`
}

type diskConfig struct {
	GatewayURL   string           `json:"gateway_url"`
	WSURL        string           `json:"ws_url"`
	ActiveUserID int64            `json:"active_user_id,omitempty"`
	Accounts     []AccountProfile `json:"accounts,omitempty"`

	// Legacy single-account fields. Kept only for one-way migration from
	// config.json written before desktop multi-account support.
	DeviceID     string      `json:"device_id,omitempty"`
	AccessToken  string      `json:"access_token,omitempty"`
	RefreshToken string      `json:"refresh_token,omitempty"`
	ExpiresAt    int64       `json:"expires_at,omitempty"`
	User         UserProfile `json:"user"`
}

type ConfigStore struct{ path string }

func NewConfigStore(dir string) *ConfigStore {
	return &ConfigStore{path: filepath.Join(dir, "config.json")}
}

func DefaultConfig() Config {
	return Config{GatewayURL: DefaultGatewayURL, WSURL: DefaultWSURL}
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
	var disk diskConfig
	if err := json.Unmarshal(b, &disk); err != nil {
		return cfg, err
	}
	cfg.GatewayURL = disk.GatewayURL
	cfg.WSURL = disk.WSURL
	cfg.ActiveUserID = disk.ActiveUserID
	cfg.Accounts = disk.Accounts
	normalizeConfig(&cfg)

	if len(cfg.Accounts) == 0 && disk.User.UserID > 0 {
		deviceID := disk.DeviceID
		if deviceID == "" {
			deviceID = uuid.NewString()
		}
		cfg.Accounts = append(cfg.Accounts, AccountProfile{
			DeviceID:     deviceID,
			AccessToken:  disk.AccessToken,
			RefreshToken: disk.RefreshToken,
			ExpiresAt:    disk.ExpiresAt,
			User:         disk.User,
			UpdatedAt:    time.Now().UnixMilli(),
		})
		if cfg.ActiveUserID == 0 {
			cfg.ActiveUserID = disk.User.UserID
		}
		cfg.LegacyCacheUserID = disk.User.UserID
	}
	normalizeConfig(&cfg)
	return cfg, nil
}

func (s *ConfigStore) Save(cfg Config) error {
	normalizeConfig(&cfg)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func normalizeConfig(cfg *Config) {
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = DefaultGatewayURL
	}
	if cfg.WSURL == "" {
		cfg.WSURL = DefaultWSURL
	}
	out := cfg.Accounts[:0]
	seen := map[int64]struct{}{}
	for _, acct := range cfg.Accounts {
		if acct.User.UserID <= 0 {
			continue
		}
		if _, ok := seen[acct.User.UserID]; ok {
			continue
		}
		if acct.DeviceID == "" {
			acct.DeviceID = uuid.NewString()
		}
		if acct.UpdatedAt == 0 {
			acct.UpdatedAt = time.Now().UnixMilli()
		}
		acct.User.Email = strings.TrimSpace(acct.User.Email)
		out = append(out, acct)
		seen[acct.User.UserID] = struct{}{}
	}
	cfg.Accounts = out
	if cfg.ActiveUserID != 0 {
		if _, ok := cfg.AccountByUserID(cfg.ActiveUserID); !ok {
			cfg.ActiveUserID = 0
		}
	}
	if cfg.ActiveUserID == 0 && len(cfg.Accounts) > 0 {
		cfg.ActiveUserID = cfg.Accounts[0].User.UserID
	}
}

func (c *Config) AccountByUserID(userID int64) (*AccountProfile, bool) {
	for i := range c.Accounts {
		if c.Accounts[i].User.UserID == userID {
			return &c.Accounts[i], true
		}
	}
	return nil, false
}

func (c *Config) AccountByEmail(email string) (*AccountProfile, bool) {
	normalized := strings.TrimSpace(strings.ToLower(email))
	if normalized == "" {
		return nil, false
	}
	for i := range c.Accounts {
		if strings.ToLower(strings.TrimSpace(c.Accounts[i].User.Email)) == normalized {
			return &c.Accounts[i], true
		}
	}
	return nil, false
}

func (c *Config) ActiveAccount() (*AccountProfile, bool) {
	if c.ActiveUserID == 0 {
		return nil, false
	}
	return c.AccountByUserID(c.ActiveUserID)
}

func (c *Config) UpsertAccount(acct AccountProfile) {
	if acct.User.UserID <= 0 {
		return
	}
	if acct.DeviceID == "" {
		acct.DeviceID = uuid.NewString()
	}
	acct.User.Email = strings.TrimSpace(acct.User.Email)
	acct.UpdatedAt = time.Now().UnixMilli()
	if existing, ok := c.AccountByUserID(acct.User.UserID); ok {
		if acct.User.Email == "" {
			acct.User.Email = existing.User.Email
		}
		if acct.User.Nickname == "" {
			acct.User.Nickname = existing.User.Nickname
		}
		if acct.User.Avatar == "" {
			acct.User.Avatar = existing.User.Avatar
		}
		*existing = acct
		return
	}
	c.Accounts = append(c.Accounts, acct)
}

func (c *Config) RemoveAccount(userID int64) bool {
	for i := range c.Accounts {
		if c.Accounts[i].User.UserID == userID {
			c.Accounts = append(c.Accounts[:i], c.Accounts[i+1:]...)
			if c.ActiveUserID == userID {
				c.ActiveUserID = 0
				if len(c.Accounts) > 0 {
					c.ActiveUserID = c.Accounts[0].User.UserID
				}
			}
			return true
		}
	}
	return false
}
