package handlers

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// fakeProvider is a Provider that records what it was asked to send. Handler
// tests must never reach a real WhatsApp backend.
type fakeProvider struct {
	mu     sync.Mutex
	name   string
	groups []provider.Group
	status provider.ProviderStatus
	msgID  string
	err    error
	sent   []sentMessage
}

type sentMessage struct {
	ChatID string
	Text   string
}

func (f *fakeProvider) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}

func (f *fakeProvider) SendText(_ context.Context, chatID, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.sent = append(f.sent, sentMessage{ChatID: chatID, Text: text})
	if f.msgID == "" {
		return "fake-message-id", nil
	}
	return f.msgID, nil
}

func (f *fakeProvider) ListGroups(context.Context) ([]provider.Group, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.groups, nil
}

func (f *fakeProvider) Status(context.Context) (provider.ProviderStatus, error) {
	if f.err != nil {
		return provider.ProviderStatus{}, f.err
	}
	return f.status, nil
}

func validGroup() map[string]any {
	return map[string]any{
		"name":     "Family",
		"chat_id":  "120363001234567890@g.us",
		"language": "he",
	}
}

func TestCreateGroupAppliesDefaults(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPost, "/api/v1/groups", validGroup())
	requireStatus(t, rec, http.StatusCreated)

	var got store.Group
	decodeInto(t, rec, &got)
	if !got.Enabled {
		t.Error("want a new group enabled by default")
	}
	if got.Threshold != nil {
		t.Errorf("threshold = %d, want nil so the global default applies", *got.Threshold)
	}
}

func TestCreateGroupRejectsPrivateChatID(t *testing.T) {
	ta := newTestAPI(t)

	payload := validGroup()
	payload["chat_id"] = "972501234567@c.us"

	rec := ta.do(t, http.MethodPost, "/api/v1/groups", payload)
	requireStatus(t, rec, http.StatusUnprocessableEntity)
	if !hasField(errorFields(t, rec), "chat_id") {
		t.Fatal("want a chat_id field error")
	}
}

func TestCreateGroupRejectsThresholdBelowOne(t *testing.T) {
	ta := newTestAPI(t)

	payload := validGroup()
	payload["threshold"] = 0

	rec := ta.do(t, http.MethodPost, "/api/v1/groups", payload)
	requireStatus(t, rec, http.StatusUnprocessableEntity)
	if !hasField(errorFields(t, rec), "threshold") {
		t.Fatal("want a threshold field error")
	}
}

func TestCreateDuplicateGroupChatIDIsConflict(t *testing.T) {
	ta := newTestAPI(t)

	requireStatus(t, ta.do(t, http.MethodPost, "/api/v1/groups", validGroup()), http.StatusCreated)

	// chat_id carries a UNIQUE constraint; a duplicate is the user's mistake,
	// not a server fault, so it must not surface as a 500.
	again := ta.do(t, http.MethodPost, "/api/v1/groups", validGroup())
	requireStatus(t, again, http.StatusConflict)
	if got := errorCode(t, again); got != codeConflict {
		t.Fatalf("code = %q, want %q", got, codeConflict)
	}
}

func TestGroupLifecycle(t *testing.T) {
	ta := newTestAPI(t)

	created := ta.do(t, http.MethodPost, "/api/v1/groups", validGroup())
	requireStatus(t, created, http.StatusCreated)
	var group store.Group
	decodeInto(t, created, &group)

	path := "/api/v1/groups/" + itoa(group.ID)

	update := validGroup()
	update["name"] = "Family & Friends"
	update["threshold"] = 5
	update["enabled"] = false
	updated := ta.do(t, http.MethodPut, path, update)
	requireStatus(t, updated, http.StatusOK)

	var after store.Group
	decodeInto(t, updated, &after)
	if after.Threshold == nil || *after.Threshold != 5 {
		t.Errorf("threshold = %v, want 5", after.Threshold)
	}
	if after.Enabled {
		t.Error("want enabled=false to be applied")
	}

	requireStatus(t, ta.do(t, http.MethodDelete, path, nil), http.StatusNoContent)
	requireStatus(t, ta.do(t, http.MethodGet, path, nil), http.StatusNotFound)
}

func TestAvailableGroupsWithoutProviderIs503(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/groups/available", nil)
	requireStatus(t, rec, http.StatusServiceUnavailable)
	if got := errorCode(t, rec); got != codeProviderUnavailable {
		t.Fatalf("code = %q, want %q", got, codeProviderUnavailable)
	}
}

func TestAvailableGroupsListsProviderGroups(t *testing.T) {
	ta := newTestAPI(t)
	ta.manager.Set(&fakeProvider{groups: []provider.Group{
		{ChatID: "120363001111111111@g.us", Name: "Family"},
		{ChatID: "120363002222222222@g.us", Name: "Work"},
	}})

	rec := ta.do(t, http.MethodGet, "/api/v1/groups/available", nil)
	requireStatus(t, rec, http.StatusOK)

	var got []provider.Group
	decodeInto(t, rec, &got)
	if len(got) != 2 || got[0].Name != "Family" {
		t.Fatalf("unexpected groups: %#v", got)
	}
}

func TestAvailableGroupsSurfacesProviderFailure(t *testing.T) {
	ta := newTestAPI(t)
	ta.manager.Set(&fakeProvider{err: errors.New("whatsapp is asleep")})

	rec := ta.do(t, http.MethodGet, "/api/v1/groups/available", nil)
	requireStatus(t, rec, http.StatusBadGateway)
	if got := errorCode(t, rec); got != codeProviderFailed {
		t.Fatalf("code = %q, want %q", got, codeProviderFailed)
	}
}

// The literal path "available" must not be parsed as an id.
func TestAvailableGroupsRouteBeatsIDRoute(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/groups/available", nil)
	if got := errorCode(t, rec); got == codeInvalidID {
		t.Fatal("the /available route was shadowed by /{id}")
	}
}
