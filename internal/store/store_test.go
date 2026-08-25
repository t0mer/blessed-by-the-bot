package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	// A real file, not :memory: — WAL semantics differ (spec §12).
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return s
}

func TestOpenAppliesMigrations(t *testing.T) {
	s := openTestStore(t)

	want := []string{
		"blessings", "contacts", "groups", "schema_migrations",
		"send_log", "settings", "wish_events", "wish_patterns",
	}
	for _, table := range want {
		var name string
		err := s.DB().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing after migration: %v", table, err)
		}
	}
}

func TestPragmasApplied(t *testing.T) {
	s := openTestStore(t)

	var journalMode string
	if err := s.DB().QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}

	var foreignKeys int
	if err := s.DB().QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	first, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	var countAfterFirst int
	if err := first.DB().QueryRow("SELECT count(*) FROM schema_migrations").Scan(&countAfterFirst); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = second.Close() }()

	var countAfterSecond int
	if err := second.DB().QueryRow("SELECT count(*) FROM schema_migrations").Scan(&countAfterSecond); err != nil {
		t.Fatal(err)
	}
	if countAfterFirst != countAfterSecond {
		t.Errorf("migration count changed on reopen: %d -> %d", countAfterFirst, countAfterSecond)
	}
	if countAfterFirst == 0 {
		t.Error("no migrations recorded")
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	s := openTestStore(t)
	_, err := s.DB().Exec(
		`INSERT INTO wish_events (group_id, sender_id, message_id, matched, created_at)
		 VALUES (99999, 'x', 'm1', 'p', '2026-08-25T00:00:00Z')`)
	if err == nil {
		t.Fatal("insert with a dangling group_id succeeded; foreign keys are not enforced")
	}
}

func TestOpenCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "test.db")
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open in a non-existent directory: %v", err)
	}
	_ = s.Close()
}
