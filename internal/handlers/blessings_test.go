package handlers

import (
	"net/http"
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
	if len(all) != 1 {
		t.Fatalf("listed %d blessings, want 1", len(all))
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

func TestListBlessingsReturnsEmptyArray(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/blessings", nil)
	requireStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("body = %q, want an empty JSON array", body)
	}
}
