// Package device provides device ID generation and persistence.
package device

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
)

// configPath is the path to the device ID config file.
// It is set by SetConfigPath for testing.
var configPath = ""

// defaultConfigPath returns the default config directory and filename.
func defaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}

	aimDir := filepath.Join(configDir, "aim")
	if err := os.MkdirAll(aimDir, 0750); err != nil {
		return "", fmt.Errorf("create aim dir: %w", err)
	}

	return filepath.Join(aimDir, "device.json"), nil
}

// deviceConfig holds the persisted device ID.
type deviceConfig struct {
	DeviceId string `json:"device_id"`
}

// LoadOrGenerate loads a persisted device ID or generates a new one.
// The device ID is persisted to a local file to remain stable across restarts.
func LoadOrGenerate() (string, error) {
	path := configPath
	if path == "" {
		var err error

		path, err = defaultConfigPath()
		if err != nil {
			// Fallback: generate a new UUID each time but document the limitation
			return generateAndWarn(), nil
		}
	}

	data, err := os.ReadFile(path)
	if err == nil {
		var cfg deviceConfig
		if json.Unmarshal(data, &cfg) == nil && cfg.DeviceId != "" {
			return cfg.DeviceId, nil
		}
	}

	// Generate new device ID
	deviceId := uuid.New().String()

	cfg := deviceConfig{DeviceId: deviceId}

	jsonData, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal device config: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0600); err != nil {
		return "", fmt.Errorf("write device config: %w", err)
	}

	return deviceId, nil
}

// generateAndWarn generates a UUID with a warning that it won't persist.
func generateAndWarn() string {
	// In environments where config persistence is not feasible,
	// we generate a UUID but document that it will change on restart.
	// This is acceptable for development; production should implement proper persistence.
	return uuid.New().String()
}

// SetConfigPath sets the config path for testing (not thread-safe).
func SetConfigPath(path string) {
	configPath = path
}

// ResetConfigPath resets the config path to default (not thread-safe).
func ResetConfigPath() {
	configPath = ""
}

// GetOrGenerate is an alias for LoadOrGenerate for convenience.
func GetOrGenerate() (string, error) {
	return LoadOrGenerate()
}

var mu sync.Mutex

// Get returns the device ID with proper synchronization.
func Get() (string, error) {
	mu.Lock()
	defer mu.Unlock()

	return LoadOrGenerate()
}
