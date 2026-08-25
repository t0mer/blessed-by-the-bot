package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"

	"github.com/t0mer/blessed-by-the-bot/internal/config"
)

func newFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.Bind(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	return fs
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("BBTB_ENCRYPTION_KEY", "")
	cfg, err := config.Load(newFlags(t, "--dev"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Port)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("data dir = %q, want ./data", cfg.DataDir)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("log level = %q, want info", cfg.LogLevel)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 1111\nlog-level: warn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BBTB_PORT", "2222")
	cfg, err := config.Load(newFlags(t, "--dev", "--config", path))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != 2222 {
		t.Errorf("port = %d, want 2222 (env beats file)", cfg.Port)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("log level = %q, want warn (from file)", cfg.LogLevel)
	}
}

func TestFlagOverridesEnv(t *testing.T) {
	t.Setenv("BBTB_PORT", "2222")
	cfg, err := config.Load(newFlags(t, "--dev", "--port", "3333"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != 3333 {
		t.Errorf("port = %d, want 3333 (flag beats env)", cfg.Port)
	}
}

func TestDataDirEnvUsesUnderscore(t *testing.T) {
	t.Setenv("BBTB_DATA_DIR", "/var/lib/bbtb")
	cfg, err := config.Load(newFlags(t, "--dev"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DataDir != "/var/lib/bbtb" {
		t.Errorf("data dir = %q, want /var/lib/bbtb", cfg.DataDir)
	}
}

func TestEncryptionKeyRequiredOutsideDev(t *testing.T) {
	t.Setenv("BBTB_ENCRYPTION_KEY", "")
	if _, err := config.Load(newFlags(t)); err == nil {
		t.Fatal("expected error when BBTB_ENCRYPTION_KEY is unset and --dev is off")
	}
}

func TestEncryptionKeyFromEnv(t *testing.T) {
	t.Setenv("BBTB_ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	cfg, err := config.Load(newFlags(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.EncryptionKey == "" {
		t.Fatal("encryption key not populated")
	}
}

func TestInvalidLogLevelRejected(t *testing.T) {
	if _, err := config.Load(newFlags(t, "--dev", "--log-level", "loud")); err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestInvalidPortRejected(t *testing.T) {
	if _, err := config.Load(newFlags(t, "--dev", "--port", "70000")); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}
