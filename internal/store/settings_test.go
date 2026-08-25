package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func TestSetAndGetSetting(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.SetSetting(ctx, "provider", "gowa"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := s.GetSetting(ctx, "provider")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "gowa" {
		t.Errorf("got %q, want gowa", got)
	}
}

func TestGetMissingSettingReturnsErrNotFound(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.GetSetting(context.Background(), "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSetSettingUpserts(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.SetSetting(ctx, "provider", "greenapi"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "provider", "gowa"); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSetting(ctx, "provider")
	if err != nil {
		t.Fatal(err)
	}
	if got != "gowa" {
		t.Errorf("got %q, want gowa", got)
	}

	var n int
	if err := s.DB().QueryRow(`SELECT count(*) FROM settings WHERE key = 'provider'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d rows for key 'provider', want 1", n)
	}
}

func TestAllSettings(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for k, v := range map[string]string{"a": "1", "b": "2"} {
		if err := s.SetSetting(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.AllSettings(ctx)
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(got) != 2 || got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %v, want map[a:1 b:2]", got)
	}
}
