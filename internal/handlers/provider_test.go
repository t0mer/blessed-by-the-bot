package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

func (f *fakeProvider) sentMessages() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMessage(nil), f.sent...)
}

func TestProviderStatusWithoutProviderIs503(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/provider/status", nil)
	requireStatus(t, rec, http.StatusServiceUnavailable)
	if got := errorCode(t, rec); got != codeProviderUnavailable {
		t.Fatalf("code = %q, want %q", got, codeProviderUnavailable)
	}
}

func TestProviderStatusReportsConnection(t *testing.T) {
	ta := newTestAPI(t)
	ta.manager.Set(&fakeProvider{name: "gowa", status: provider.ProviderStatus{
		Provider: "gowa", Connected: true, State: "authorized",
	}})

	rec := ta.do(t, http.MethodGet, "/api/v1/provider/status", nil)
	requireStatus(t, rec, http.StatusOK)

	var got provider.ProviderStatus
	decodeInto(t, rec, &got)
	if !got.Connected || got.State != "authorized" {
		t.Fatalf("unexpected status: %#v", got)
	}
}

// A backend that needs a QR scan is a normal, reportable state — not an error.
// The dashboard renders it as a call to action, so it must arrive as a 200.
func TestProviderStatusReportsQRNeededAsSuccess(t *testing.T) {
	ta := newTestAPI(t)
	ta.manager.Set(&fakeProvider{status: provider.ProviderStatus{
		Provider: "gowa", Connected: false, State: "notLoggedIn", NeedsQR: true,
	}})

	rec := ta.do(t, http.MethodGet, "/api/v1/provider/status", nil)
	requireStatus(t, rec, http.StatusOK)

	var got provider.ProviderStatus
	decodeInto(t, rec, &got)
	if !got.NeedsQR {
		t.Fatal("want needs_qr surfaced to the UI")
	}
}

func TestProviderStatusSurfacesBackendFailure(t *testing.T) {
	ta := newTestAPI(t)
	ta.manager.Set(&fakeProvider{err: errors.New("connection refused")})

	rec := ta.do(t, http.MethodGet, "/api/v1/provider/status", nil)
	requireStatus(t, rec, http.StatusBadGateway)
	if got := errorCode(t, rec); got != codeProviderFailed {
		t.Fatalf("code = %q, want %q", got, codeProviderFailed)
	}
}

func TestProviderTestSendsToNormalizedChatID(t *testing.T) {
	fake := &fakeProvider{msgID: "BAE5F4"}
	ta := newTestAPI(t)
	ta.manager.Set(fake)

	rec := ta.do(t, http.MethodPost, "/api/v1/provider/test", map[string]any{
		"phone": "+972 50-123 4567",
	})
	requireStatus(t, rec, http.StatusOK)

	var got providerTestResponse
	decodeInto(t, rec, &got)
	if got.MessageID != "BAE5F4" {
		t.Errorf("message_id = %q, want BAE5F4", got.MessageID)
	}
	if got.ChatID != "972501234567@c.us" {
		t.Errorf("chat_id = %q, want the normalized private chat id", got.ChatID)
	}

	sent := fake.sentMessages()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sent))
	}
	if sent[0].ChatID != "972501234567@c.us" {
		t.Errorf("provider received chat id %q", sent[0].ChatID)
	}
	if sent[0].Text == "" {
		t.Error("want a default test message when none is supplied")
	}
}

func TestProviderTestUsesSuppliedMessage(t *testing.T) {
	fake := &fakeProvider{}
	ta := newTestAPI(t)
	ta.manager.Set(fake)

	requireStatus(t, ta.do(t, http.MethodPost, "/api/v1/provider/test", map[string]any{
		"phone":   "972501234567",
		"message": "hello from the bot",
	}), http.StatusOK)

	sent := fake.sentMessages()
	if len(sent) != 1 || sent[0].Text != "hello from the bot" {
		t.Fatalf("unexpected sends: %#v", sent)
	}
}

func TestProviderTestAcceptsAGroupChatID(t *testing.T) {
	fake := &fakeProvider{}
	ta := newTestAPI(t)
	ta.manager.Set(fake)

	requireStatus(t, ta.do(t, http.MethodPost, "/api/v1/provider/test", map[string]any{
		"phone": "120363001234567890@g.us",
	}), http.StatusOK)

	sent := fake.sentMessages()
	if len(sent) != 1 || sent[0].ChatID != "120363001234567890@g.us" {
		t.Fatalf("a full chat id must pass through untouched, got %#v", sent)
	}
}

func TestProviderTestRejectsBadPhone(t *testing.T) {
	ta := newTestAPI(t)
	ta.manager.Set(&fakeProvider{})

	rec := ta.do(t, http.MethodPost, "/api/v1/provider/test", map[string]any{"phone": "123"})
	requireStatus(t, rec, http.StatusUnprocessableEntity)
	if !hasField(errorFields(t, rec), "phone") {
		t.Fatal("want a phone field error")
	}
}

func TestProviderTestWithoutProviderIs503(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPost, "/api/v1/provider/test", map[string]any{"phone": "972501234567"})
	requireStatus(t, rec, http.StatusServiceUnavailable)
}

func TestProviderTestReportsSendFailure(t *testing.T) {
	ta := newTestAPI(t)
	ta.manager.Set(&fakeProvider{err: errors.New("401 unauthorized")})

	rec := ta.do(t, http.MethodPost, "/api/v1/provider/test", map[string]any{"phone": "972501234567"})
	requireStatus(t, rec, http.StatusBadGateway)
	if got := errorCode(t, rec); got != codeProviderFailed {
		t.Fatalf("code = %q, want %q", got, codeProviderFailed)
	}
}
