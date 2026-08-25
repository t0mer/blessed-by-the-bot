package crypto_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
)

func TestLoadOrCreateDevKeyGeneratesThenReuses(t *testing.T) {
	dir := t.TempDir()

	first, err := crypto.LoadOrCreateDevKey(dir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := crypto.New(first); err != nil {
		t.Fatalf("generated key is unusable: %v", err)
	}

	second, err := crypto.LoadOrCreateDevKey(dir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Error("key changed between calls; dev settings would not survive a restart")
	}
}

func TestDevKeyFileIsNotWorldReadable(t *testing.T) {
	dir := t.TempDir()
	if _, err := crypto.LoadOrCreateDevKey(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, crypto.DevKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("dev key mode = %o, want 600", perm)
	}
}

func TestLoadOrCreateDevKeyRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, crypto.DevKeyFile), []byte("not-a-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := crypto.LoadOrCreateDevKey(dir); err == nil {
		t.Fatal("corrupt dev key accepted")
	}
}

func TestLoadOrCreateDevKeyCreatesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does", "not", "exist")
	if _, err := crypto.LoadOrCreateDevKey(dir); err != nil {
		t.Fatalf("missing dir: %v", err)
	}
}
