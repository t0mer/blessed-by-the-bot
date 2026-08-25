package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func TestSeededPatternsPresent(t *testing.T) {
	s := openTestStore(t)
	got, err := s.ListWishPatterns(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) < 17 {
		t.Fatalf("got %d seeded patterns, want at least 17", len(got))
	}

	seen := make(map[string]bool, len(got))
	for _, p := range got {
		seen[p.Pattern] = true
	}
	for _, want := range []string{"מזל טוב", "happy birthday", "🎂", "יום הולדת שמח"} {
		if !seen[want] {
			t.Errorf("seed is missing the pattern %q", want)
		}
	}
}

func TestSeededPatternsNotDuplicatedOnReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	first, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	before, err := first.ListWishPatterns(ctx)
	if err != nil {
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

	after, err := second.ListWishPatterns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Errorf("pattern count changed on reopen: %d -> %d", len(before), len(after))
	}
}

func TestListEnabledWishPatternsExcludesDisabled(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	all, err := s.ListWishPatterns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	victim := all[0]
	victim.Enabled = false
	if _, err := s.UpdateWishPattern(ctx, &victim); err != nil {
		t.Fatalf("disable pattern: %v", err)
	}

	enabled, err := s.ListEnabledWishPatterns(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != len(all)-1 {
		t.Errorf("got %d enabled patterns, want %d", len(enabled), len(all)-1)
	}
	for _, p := range enabled {
		if p.ID == victim.ID {
			t.Errorf("disabled pattern %d was returned", victim.ID)
		}
	}
}

func TestCreateWishPatternRejectsDuplicate(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	p := &store.WishPattern{Language: "he", Pattern: "מזל טוב", Enabled: true}
	if _, err := s.CreateWishPattern(ctx, p); err == nil {
		t.Fatal("duplicate (language, pattern) accepted; UNIQUE constraint is not enforced")
	}
}

func TestCreateWishPatternRoundTrip(t *testing.T) {
	s := openTestStore(t)
	got, err := s.CreateWishPattern(context.Background(),
		&store.WishPattern{Language: "ru", Pattern: "с днем рождения", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ID == 0 || got.Pattern != "с днем рождения" || !got.Enabled {
		t.Errorf("round trip changed fields: %+v", got)
	}
}

func TestUpdateWishPatternTogglesEnabled(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateWishPattern(ctx,
		&store.WishPattern{Language: "ru", Pattern: "поздравляю", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created.Enabled = false
	got, err := s.UpdateWishPattern(ctx, created)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Enabled {
		t.Error("pattern still enabled after update")
	}
}

func TestDeleteWishPattern(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateWishPattern(ctx,
		&store.WishPattern{Language: "ru", Pattern: "ура", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.DeleteWishPattern(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteWishPattern(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
}
