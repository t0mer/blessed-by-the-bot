package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// seedSendLog writes n entries of the given kind straight through the store,
// which is the layer the history endpoint reads.
func seedSendLog(t *testing.T, st *store.Store, kind string, n int) {
	t.Helper()
	for range n {
		if _, err := st.AppendSendLog(context.Background(), &store.SendLogEntry{
			Kind:     kind,
			Provider: "fake",
			ChatID:   "972501234567@c.us",
			Status:   store.StatusSent,
		}); err != nil {
			t.Fatalf("seeding send log: %v", err)
		}
	}
}

func TestHistoryReturnsEmptyArrayWhenNothingSent(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/history", nil)
	requireStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("body = %q, want an empty JSON array", body)
	}
}

func TestHistoryFiltersByKind(t *testing.T) {
	ta := newTestAPI(t)
	seedSendLog(t, ta.store, store.KindScheduled, 2)
	seedSendLog(t, ta.store, store.KindGroupEcho, 3)

	rec := ta.do(t, http.MethodGet, "/api/v1/history?kind=group_echo", nil)
	requireStatus(t, rec, http.StatusOK)

	var got []store.SendLogEntry
	decodeInto(t, rec, &got)
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	for _, e := range got {
		if e.Kind != store.KindGroupEcho {
			t.Fatalf("kind = %q, want only group_echo", e.Kind)
		}
	}
}

func TestHistoryHonoursLimit(t *testing.T) {
	ta := newTestAPI(t)
	seedSendLog(t, ta.store, store.KindScheduled, 5)

	rec := ta.do(t, http.MethodGet, "/api/v1/history?limit=2", nil)
	requireStatus(t, rec, http.StatusOK)

	var got []store.SendLogEntry
	decodeInto(t, rec, &got)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
}

func TestHistoryRejectsUnknownKind(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/history?kind=telepathy", nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if got := errorCode(t, rec); got != codeInvalidQuery {
		t.Fatalf("code = %q, want %q", got, codeInvalidQuery)
	}
}

func TestHistoryRejectsNonNumericLimit(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/history?limit=lots", nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if got := errorCode(t, rec); got != codeInvalidQuery {
		t.Fatalf("code = %q, want %q", got, codeInvalidQuery)
	}
}

// fakeSender stands in for the Phase 5 blessing engine.
type fakeSender struct {
	entry  *store.SendLogEntry
	err    error
	callID int64
	force  bool
	calls  int
}

func (f *fakeSender) SendNow(_ context.Context, contactID int64, force bool) (*store.SendLogEntry, error) {
	f.calls++
	f.callID = contactID
	f.force = force
	return f.entry, f.err
}

func TestSendNowWithoutEngineIs501(t *testing.T) {
	ta := newTestAPI(t)

	created := ta.do(t, http.MethodPost, "/api/v1/contacts", validContact())
	requireStatus(t, created, http.StatusCreated)
	var contact store.Contact
	decodeInto(t, created, &contact)

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts/"+itoa(contact.ID)+"/send-now", nil)
	requireStatus(t, rec, http.StatusNotImplemented)
	if got := errorCode(t, rec); got != codeNotImplemented {
		t.Fatalf("code = %q, want %q", got, codeNotImplemented)
	}
}

func TestSendNowDelegatesToTheEngine(t *testing.T) {
	sender := &fakeSender{entry: &store.SendLogEntry{
		Kind: store.KindScheduled, Provider: "fake", ChatID: "972501234567@c.us", Status: store.StatusSent,
	}}
	ta := newTestAPI(t, func(d *Deps) { d.Sender = sender })

	created := ta.do(t, http.MethodPost, "/api/v1/contacts", validContact())
	var contact store.Contact
	decodeInto(t, created, &contact)

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts/"+itoa(contact.ID)+"/send-now", nil)
	requireStatus(t, rec, http.StatusOK)

	if sender.calls != 1 || sender.callID != contact.ID {
		t.Fatalf("engine called %d times with id %d, want 1 call with %d", sender.calls, sender.callID, contact.ID)
	}
	if sender.force {
		t.Error("want force=false by default so the yearly dedupe still applies")
	}
}

func TestSendNowPassesForceFlag(t *testing.T) {
	sender := &fakeSender{entry: &store.SendLogEntry{Status: store.StatusSent}}
	ta := newTestAPI(t, func(d *Deps) { d.Sender = sender })

	created := ta.do(t, http.MethodPost, "/api/v1/contacts", validContact())
	var contact store.Contact
	decodeInto(t, created, &contact)

	requireStatus(t, ta.do(t, http.MethodPost,
		"/api/v1/contacts/"+itoa(contact.ID)+"/send-now?force=true", nil), http.StatusOK)

	if !sender.force {
		t.Fatal("want force=true forwarded to the engine")
	}
}

func TestSendNowRejectsBadForceValue(t *testing.T) {
	ta := newTestAPI(t, func(d *Deps) { d.Sender = &fakeSender{} })

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts/1/send-now?force=maybe", nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if got := errorCode(t, rec); got != codeInvalidQuery {
		t.Fatalf("code = %q, want %q", got, codeInvalidQuery)
	}
}

func TestSendNowPropagatesMissingContact(t *testing.T) {
	ta := newTestAPI(t, func(d *Deps) { d.Sender = &fakeSender{err: store.ErrNotFound} })

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts/9999/send-now", nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestSendNowReportsEngineFailure(t *testing.T) {
	ta := newTestAPI(t, func(d *Deps) {
		d.Sender = &fakeSender{err: errors.New("no blessing matches this contact")}
	})

	rec := ta.do(t, http.MethodPost, "/api/v1/contacts/1/send-now", nil)
	requireStatus(t, rec, http.StatusInternalServerError)
}
