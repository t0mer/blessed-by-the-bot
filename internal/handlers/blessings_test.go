package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func validBlessing() map[string]any {
	return map[string]any{
		"event_type": "birthday",
		"language":   "he",
		"text":       "יום הולדת שמח {{name}}!",
	}
}

func TestCreateBlessingLeavesTargetingUnsetByDefault(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPost, "/api/v1/blessings", validBlessing())
	requireStatus(t, rec, http.StatusCreated)

	var got store.Blessing
	decodeInto(t, rec, &got)
	if got.Gender != nil {
		t.Errorf("gender = %q, want nil meaning any", *got.Gender)
	}
	if got.Relation != nil {
		t.Errorf("relation = %q, want nil meaning any", *got.Relation)
	}
	if !got.Enabled {
		t.Error("want a new blessing enabled by default")
	}
}

func TestCreateBlessingCollapsesBlankTargetingToNull(t *testing.T) {
	ta := newTestAPI(t)

	payload := validBlessing()
	payload["gender"] = ""
	payload["relation"] = ""

	rec := ta.do(t, http.MethodPost, "/api/v1/blessings", payload)
	requireStatus(t, rec, http.StatusCreated)

	var got store.Blessing
	decodeInto(t, rec, &got)
	if got.Gender != nil || got.Relation != nil {
		t.Fatalf("blank targeting must become NULL, got gender=%v relation=%v", got.Gender, got.Relation)
	}
}

func TestCreateBlessingHonoursTargeting(t *testing.T) {
	ta := newTestAPI(t)

	payload := validBlessing()
	payload["gender"] = "female"
	payload["relation"] = "close_friend"

	rec := ta.do(t, http.MethodPost, "/api/v1/blessings", payload)
	requireStatus(t, rec, http.StatusCreated)

	var got store.Blessing
	decodeInto(t, rec, &got)
	if got.Gender == nil || *got.Gender != store.GenderFemale {
		t.Errorf("gender = %v, want female", got.Gender)
	}
	if got.Relation == nil || *got.Relation != store.RelationCloseFriend {
		t.Errorf("relation = %v, want close_friend", got.Relation)
	}
}

func TestCreateBlessingRejectsBadTargeting(t *testing.T) {
	ta := newTestAPI(t)

	payload := validBlessing()
	payload["gender"] = "robot"
	payload["relation"] = "nemesis"
	payload["text"] = "   "

	rec := ta.do(t, http.MethodPost, "/api/v1/blessings", payload)
	requireStatus(t, rec, http.StatusUnprocessableEntity)

	fields := errorFields(t, rec)
	for _, want := range []string{"gender", "relation", "text"} {
		if !hasField(fields, want) {
			t.Errorf("missing field error for %q; got %#v", want, fields)
		}
	}
}

func TestBlessingLifecycle(t *testing.T) {
	ta := newTestAPI(t)

	created := ta.do(t, http.MethodPost, "/api/v1/blessings", validBlessing())
	requireStatus(t, created, http.StatusCreated)
	var blessing store.Blessing
	decodeInto(t, created, &blessing)

	path := "/api/v1/blessings/" + itoa(blessing.ID)

	listed := ta.do(t, http.MethodGet, "/api/v1/blessings", nil)
	requireStatus(t, listed, http.StatusOK)
	var all []store.Blessing
	decodeInto(t, listed, &all)
	// The starter seed is also in here, so look for the new row rather than
	// assuming it is the only one.
	found := false
	for _, b := range all {
		if b.ID == blessing.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the created blessing is missing from the list of %d", len(all))
	}

	update := validBlessing()
	update["text"] = "מזל טוב {{name}}"
	update["enabled"] = false
	updated := ta.do(t, http.MethodPut, path, update)
	requireStatus(t, updated, http.StatusOK)

	var after store.Blessing
	decodeInto(t, updated, &after)
	if after.Enabled {
		t.Error("want enabled=false to be applied")
	}
	if after.Text != "מזל טוב {{name}}" {
		t.Errorf("text = %q, not updated", after.Text)
	}

	requireStatus(t, ta.do(t, http.MethodDelete, path, nil), http.StatusNoContent)
	requireStatus(t, ta.do(t, http.MethodGet, path, nil), http.StatusNotFound)
}

// Migration 0003 seeds starter templates so a fresh install can actually send.
// The UI must show them on first run rather than an empty editor.
func TestListBlessingsReturnsTheSeededStarterSet(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/blessings", nil)
	requireStatus(t, rec, http.StatusOK)

	var got []store.Blessing
	decodeInto(t, rec, &got)
	if len(got) == 0 {
		t.Fatal("want the seeded starter blessings, got none")
	}

	// Every event type needs cover, or a contact of that type has nothing to send.
	types := map[string]bool{}
	languages := map[string]bool{}
	nameFree := map[string]bool{}
	for _, b := range got {
		types[b.EventType] = true
		languages[b.Language] = true
		if !strings.Contains(b.Text, "{{name}}") {
			nameFree[b.EventType] = true
		}
	}
	for _, want := range []string{"birthday", "wedding", "anniversary", "custom"} {
		if !types[want] {
			t.Errorf("no seeded template for event type %q", want)
		}
	}
	for _, want := range []string{"he", "en"} {
		if !languages[want] {
			t.Errorf("no seeded template for language %q", want)
		}
	}
	// Group echo can only use name-free templates; without one it can never fire.
	for _, want := range []string{"birthday", "wedding", "anniversary"} {
		if !nameFree[want] {
			t.Errorf("no name-free seeded template for %q; group echo would have nothing to send", want)
		}
	}
}

// Hebrew greetings are grammatically gendered, so the seed must carry both forms
// or half the contacts fall back to a neutral template.
func TestSeededHebrewBirthdaysCoverBothGenders(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/blessings", nil)
	requireStatus(t, rec, http.StatusOK)

	var got []store.Blessing
	decodeInto(t, rec, &got)

	genders := map[string]bool{}
	for _, b := range got {
		if b.EventType == store.EventBirthday && b.Language == "he" && b.Gender != nil {
			genders[*b.Gender] = true
		}
	}
	for _, want := range []string{store.GenderMale, store.GenderFemale} {
		if !genders[want] {
			t.Errorf("no seeded Hebrew birthday template for gender %q", want)
		}
	}
}
