package handlers

import (
	"net/http"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func TestListWishPatternsReturnsTheSeededSet(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/wish-patterns", nil)
	requireStatus(t, rec, http.StatusOK)

	var got []store.WishPattern
	decodeInto(t, rec, &got)
	// Migration 0002 seeds Hebrew, English and emoji patterns; the UI must show
	// them on first run rather than an empty editor.
	if len(got) == 0 {
		t.Fatal("want the seeded wish patterns, got none")
	}

	languages := map[string]bool{}
	for _, p := range got {
		languages[p.Language] = true
	}
	for _, want := range []string{"he", "en"} {
		if !languages[want] {
			t.Errorf("no seeded pattern for language %q", want)
		}
	}
}

func TestCreateWishPatternAcceptsPlainSubstring(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPost, "/api/v1/wish-patterns", map[string]any{
		"language": "en",
		"pattern":  "wishing you the very best",
	})
	requireStatus(t, rec, http.StatusCreated)

	var got store.WishPattern
	decodeInto(t, rec, &got)
	if got.ID == 0 || !got.Enabled {
		t.Fatalf("unexpected pattern: %#v", got)
	}
}

func TestCreateWishPatternAcceptsValidRegex(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPost, "/api/v1/wish-patterns", map[string]any{
		"language": "en",
		"pattern":  `/happy\s+b(-)?day/`,
	})
	requireStatus(t, rec, http.StatusCreated)
}

func TestCreateWishPatternRejectsBrokenRegex(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPost, "/api/v1/wish-patterns", map[string]any{
		"language": "en",
		"pattern":  "/[unclosed/",
	})
	requireStatus(t, rec, http.StatusUnprocessableEntity)
	if !hasField(errorFields(t, rec), "pattern") {
		t.Fatal("want a pattern field error")
	}
}

func TestWishPatternUpdateAndDelete(t *testing.T) {
	ta := newTestAPI(t)

	created := ta.do(t, http.MethodPost, "/api/v1/wish-patterns", map[string]any{
		"language": "he",
		"pattern":  "איחולים חמים",
	})
	requireStatus(t, created, http.StatusCreated)
	var pattern store.WishPattern
	decodeInto(t, created, &pattern)

	path := "/api/v1/wish-patterns/" + itoa(pattern.ID)

	updated := ta.do(t, http.MethodPut, path, map[string]any{
		"language": "he",
		"pattern":  "איחולים חמים ולבביים",
		"enabled":  false,
	})
	requireStatus(t, updated, http.StatusOK)

	var after store.WishPattern
	decodeInto(t, updated, &after)
	if after.Enabled {
		t.Error("want enabled=false to be applied")
	}
	if after.Pattern != "איחולים חמים ולבביים" {
		t.Errorf("pattern = %q, not updated", after.Pattern)
	}

	requireStatus(t, ta.do(t, http.MethodDelete, path, nil), http.StatusNoContent)
	requireStatus(t, ta.do(t, http.MethodDelete, path, nil), http.StatusNotFound)
}

// Spec §8 lists no fetch-by-id for wish patterns; the UI edits from the list.
func TestWishPatternHasNoGetByID(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/wish-patterns/1", nil)
	requireStatus(t, rec, http.StatusMethodNotAllowed)
}

// (language, pattern) is UNIQUE, and migration 0002 seeds a set of defaults, so
// re-adding a seeded pattern is a user mistake, not a server fault.
func TestCreateDuplicateWishPatternIsConflict(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPost, "/api/v1/wish-patterns", map[string]any{
		"language": "he",
		"pattern":  "מזל טוב", // seeded by migration 0002
	})
	requireStatus(t, rec, http.StatusConflict)
	if got := errorCode(t, rec); got != codeConflict {
		t.Fatalf("code = %q, want %q", got, codeConflict)
	}
}
