package device

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGeneratePersistsDeviceID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	SetConfigPath(path)

	defer ResetConfigPath()

	first, err := LoadOrGenerate()
	if err != nil {
		t.Fatalf("LoadOrGenerate first returned error: %v", err)
	}

	if first == "" {
		t.Fatal("first device id is empty")
	}

	second, err := LoadOrGenerate()
	if err != nil {
		t.Fatalf("LoadOrGenerate second returned error: %v", err)
	}

	if second != first {
		t.Fatalf("second device id = %s, want %s", second, first)
	}
}

func TestLoadOrGenerateOverwritesInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	if err := os.WriteFile(path, []byte(`{"device_id":""}`), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	SetConfigPath(path)

	defer ResetConfigPath()

	deviceID, err := LoadOrGenerate()
	if err != nil {
		t.Fatalf("LoadOrGenerate returned error: %v", err)
	}

	if deviceID == "" {
		t.Fatal("device id is empty")
	}
}

func TestGetUsesSynchronizedLoader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	SetConfigPath(path)

	defer ResetConfigPath()

	deviceID, err := Get()
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	if deviceID == "" {
		t.Fatal("device id is empty")
	}
}

func TestLoadOrGenerateReturnsWriteError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "device.json")
	SetConfigPath(path)

	defer ResetConfigPath()

	if _, err := LoadOrGenerate(); err == nil {
		t.Fatal("LoadOrGenerate returned nil error")
	}
}
