package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigStoreMigratesLegacySingleAccount(t *testing.T) {
	dir := t.TempDir()
	legacy := map[string]any{
		"gateway_url":   "http://gw",
		"ws_url":        "ws://gw/ws",
		"device_id":     "legacy-device",
		"access_token":  "access-1",
		"refresh_token": "refresh-1",
		"expires_at":    float64(123),
		"user": map[string]any{
			"user_id":  float64(42),
			"email":    "a@example.com",
			"nickname": "alice",
			"avatar":   "avatar-a",
		},
	}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewConfigStore(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveUserID != 42 {
		t.Fatalf("ActiveUserID = %d, want 42", cfg.ActiveUserID)
	}
	if len(cfg.Accounts) != 1 {
		t.Fatalf("len(Accounts) = %d, want 1", len(cfg.Accounts))
	}
	if cfg.LegacyCacheUserID != 42 {
		t.Fatalf("LegacyCacheUserID = %d, want 42", cfg.LegacyCacheUserID)
	}
	acct := cfg.Accounts[0]
	if acct.DeviceID != "legacy-device" || acct.AccessToken != "access-1" || acct.RefreshToken != "refresh-1" || acct.ExpiresAt != 123 {
		t.Fatalf("unexpected migrated account: %+v", acct)
	}
	if acct.User.Email != "a@example.com" || acct.User.Nickname != "alice" || acct.User.Avatar != "avatar-a" {
		t.Fatalf("unexpected migrated user: %+v", acct.User)
	}
}

func TestConfigStoreSavesAccountsWithoutLegacyRootTokens(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.UpsertAccount(AccountProfile{
		DeviceID:     "device-a",
		AccessToken:  "access-a",
		RefreshToken: "refresh-a",
		ExpiresAt:    456,
		User:         UserProfile{UserID: 7, Email: "b@example.com"},
	})
	cfg.ActiveUserID = 7

	if err := NewConfigStore(dir).Save(cfg); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- test reads a config path under t.TempDir().
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"device_id", "access_token", "refresh_token", "expires_at", "user"} {
		if _, ok := raw[k]; ok {
			t.Fatalf("legacy root field %q was written: %s", k, string(data))
		}
	}
	if _, ok := raw["accounts"]; !ok {
		t.Fatalf("accounts missing: %s", string(data))
	}
}
