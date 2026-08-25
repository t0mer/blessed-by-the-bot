package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// itoa formats a row id for URL building.
func itoa(id int64) string { return strconv.FormatInt(id, 10) }

// errorFields extracts the validation field list from a response.
func errorFields(t *testing.T, rec *httptest.ResponseRecorder) []FieldError {
	t.Helper()
	var env errorEnvelope
	decodeInto(t, rec, &env)
	return env.Error.Fields
}

// validContact is a well-formed create payload; tests copy and spoil one field.
func validContact() map[string]any {
	return map[string]any{
		"name":       "Dana",
		"phone":      "+972 50-123 4567",
		"event_date": "1990-05-17",
		"event_type": "birthday",
		"language":   "he",
		"relation":   "friend",
		"gender":     "female",
	}
}

func TestCreateContactAppliesDefaultsAndNormalizesPhone(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts", validContact())
	requireStatus(t, rec, http.StatusCreated)

	var got store.Contact
	decodeInto(t, rec, &got)

	if got.ID == 0 {
		t.Fatal("want a generated id")
	}
	if got.Phone != "972501234567" {
		t.Errorf("phone = %q, want the punctuation stripped", got.Phone)
	}
	if got.Importance != 3 {
		t.Errorf("importance = %d, want the default 3", got.Importance)
	}
	if !got.Enabled {
		t.Error("want a new contact to be enabled by default")
	}
	if got.SendTime != nil {
		t.Errorf("send_time = %v, want nil so the general setting applies", *got.SendTime)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("want timestamps populated by the store")
	}
}

func TestCreateContactHonoursExplicitOptionals(t *testing.T) {
	ta := newTestAPI(t)

	payload := validContact()
	payload["importance"] = 5
	payload["enabled"] = false
	payload["send_time"] = "07:30"

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts", payload)
	requireStatus(t, rec, http.StatusCreated)

	var got store.Contact
	decodeInto(t, rec, &got)
	if got.Importance != 5 {
		t.Errorf("importance = %d, want 5", got.Importance)
	}
	if got.Enabled {
		t.Error("want enabled=false to be honoured")
	}
	if got.SendTime == nil || *got.SendTime != "07:30" {
		t.Errorf("send_time = %v, want 07:30", got.SendTime)
	}
}

func TestCreateContactReportsEveryInvalidField(t *testing.T) {
	ta := newTestAPI(t)

	payload := validContact()
	payload["name"] = ""
	payload["phone"] = "123"
	payload["event_type"] = "graduation"
	payload["gender"] = "unknown"
	payload["event_date"] = "17/05/1990"

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts", payload)
	requireStatus(t, rec, http.StatusUnprocessableEntity)
	if got := errorCode(t, rec); got != codeValidationFailed {
		t.Fatalf("code = %q, want %q", got, codeValidationFailed)
	}

	fields := errorFields(t, rec)
	for _, want := range []string{"name", "phone", "event_type", "gender", "event_date"} {
		if !hasField(fields, want) {
			t.Errorf("missing field error for %q; got %#v", want, fields)
		}
	}
}

func TestCreateContactRejectsUnknownField(t *testing.T) {
	ta := newTestAPI(t)

	payload := validContact()
	payload["nickname"] = "Dan"

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts", payload)
	requireStatus(t, rec, http.StatusBadRequest)
	if got := errorCode(t, rec); got != codeInvalidJSON {
		t.Fatalf("code = %q, want %q", got, codeInvalidJSON)
	}
}

func TestListContactsReturnsEmptyArrayNotNull(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/contacts", nil)
	requireStatus(t, rec, http.StatusOK)

	// The SPA maps over this; a JSON null would crash it.
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("body = %q, want an empty JSON array", body)
	}
}

func TestContactLifecycle(t *testing.T) {
	ta := newTestAPI(t)

	created := ta.do(t, http.MethodPost, "/api/v1/contacts", validContact())
	requireStatus(t, created, http.StatusCreated)
	var contact store.Contact
	decodeInto(t, created, &contact)

	path := "/api/v1/contacts/" + itoa(contact.ID)

	fetched := ta.do(t, http.MethodGet, path, nil)
	requireStatus(t, fetched, http.StatusOK)

	update := validContact()
	update["name"] = "Dana Cohen"
	update["relation"] = "family"
	updated := ta.do(t, http.MethodPut, path, update)
	requireStatus(t, updated, http.StatusOK)

	var after store.Contact
	decodeInto(t, updated, &after)
	if after.Name != "Dana Cohen" || after.Relation != store.RelationFamily {
		t.Fatalf("update not applied: %#v", after)
	}
	if after.ID != contact.ID {
		t.Fatalf("id changed on update: %d -> %d", contact.ID, after.ID)
	}

	deleted := ta.do(t, http.MethodDelete, path, nil)
	requireStatus(t, deleted, http.StatusNoContent)

	gone := ta.do(t, http.MethodGet, path, nil)
	requireStatus(t, gone, http.StatusNotFound)
	if got := errorCode(t, gone); got != codeNotFound {
		t.Fatalf("code = %q, want %q", got, codeNotFound)
	}
}

func TestContactUpdateOfMissingRowIs404(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPut, "/api/v1/contacts/9999", validContact())
	requireStatus(t, rec, http.StatusNotFound)
}

func TestContactDeleteOfMissingRowIs404(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodDelete, "/api/v1/contacts/9999", nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestContactBadIDIs400(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/contacts/abc", nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if got := errorCode(t, rec); got != codeInvalidID {
		t.Fatalf("code = %q, want %q", got, codeInvalidID)
	}
}
