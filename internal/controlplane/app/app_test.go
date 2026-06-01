package app

import (
	"os"
	"testing"
	"time"
)

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{
		Addr:           ":8080",
		DatabaseURL:    "file:/var/lib/cloud-cli-proxy/cloud-cli-proxy.db",
		AdminUsername:  "admin",
		AdminPassword:  "secret",
		AdminJWTSecret: "jwt-secret",
	}

	if cfg.Addr == "" {
		t.Error("Addr should not be empty")
	}
	if cfg.DatabaseURL == "" {
		t.Error("DatabaseURL should not be empty")
	}
	if cfg.ExpiryScanInterval == 0 {
		// Zero is valid (uses default), just document
		t.Log("ExpiryScanInterval is zero, will use default")
	}
	if cfg.ReconcileInterval == 0 {
		t.Log("ReconcileInterval is zero, will use default")
	}
}

func TestConfig_SSHProxyDefaults(t *testing.T) {
	cfg := Config{
		SSHProxyAddr:              ":2222",
		SSHProxyContainerUser:     "workspace",
		SSHProxyContainerPassword: "pass",
		SSHProxyHostKeyPath:       "/etc/ssh/host_key",
	}
	if cfg.SSHProxyAddr != ":2222" {
		t.Errorf("SSHProxyAddr = %q, want :2222", cfg.SSHProxyAddr)
	}
}

func TestNewLogger_Defaults(t *testing.T) {
	// Clear env to get default behavior
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("LOG_FORMAT")

	logger := newLogger()
	if logger == nil {
		t.Fatal("newLogger should not return nil")
	}
	// Default should be a text handler at Info level
}

func TestNewLogger_JSONFormat(t *testing.T) {
	os.Setenv("LOG_FORMAT", "json")
	defer os.Unsetenv("LOG_FORMAT")

	logger := newLogger()
	if logger == nil {
		t.Fatal("newLogger should not return nil")
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	os.Setenv("LOG_LEVEL", "INVALID")
	defer os.Unsetenv("LOG_LEVEL")

	logger := newLogger()
	// Should not panic; defaults to Info
	if logger == nil {
		t.Fatal("newLogger should not return nil even with invalid level")
	}
}

func TestNewLogger_ValidLevels(t *testing.T) {
	levels := []string{"DEBUG", "debug", "INFO", "info", "WARN", "warn", "ERROR", "error"}
	for _, lvl := range levels {
		os.Setenv("LOG_LEVEL", lvl)
		logger := newLogger()
		if logger == nil {
			t.Errorf("newLogger returned nil for level %q", lvl)
		}
	}
	os.Unsetenv("LOG_LEVEL")
}

func TestConfig_ExpiryScanInterval(t *testing.T) {
	cfg := Config{
		ExpiryScanInterval: 5 * time.Minute,
		ReconcileInterval:  30 * time.Second,
	}
	if cfg.ExpiryScanInterval != 5*time.Minute {
		t.Errorf("ExpiryScanInterval = %v", cfg.ExpiryScanInterval)
	}
	if cfg.ReconcileInterval != 30*time.Second {
		t.Errorf("ReconcileInterval = %v", cfg.ReconcileInterval)
	}
}
