package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/gowa"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
)

// recordingHandler stands in for the Phase 6 echo engine.
type recordingHandler struct {
	mu  sync.Mutex
	got []provider.IncomingMessage
	err error
}

func (h *recordingHandler) HandleIncoming(_ context.Context, msg provider.IncomingMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.got = append(h.got, msg)
	return h.err
}

func (h *recordingHandler) messages() []provider.IncomingMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]provider.IncomingMessage(nil), h.got...)
}

// saveSettings persists a configuration through the real settings service.
func saveSettings(t *testing.T, ta *testAPI, mutate func(*settings.Settings)) {
	t.Helper()
	s := settings.Defaults()
	mutate(s)
	if err := ta.settings.Save(context.Background(), s); err != nil {
		t.Fatalf("saving settings: %v", err)
	}
}

// postRaw sends a body with explicit headers, which the signed-webhook tests need.
func (ta *testAPI) postRaw(t *testing.T, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	ta.router.ServeHTTP(rec, req)
	return rec
}

func signGOWA(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

const greenAPIMessageBody = `{
  "typeWebhook": "incomingMessageReceived",
  "instanceData": {"idInstance": 7103123456, "wid": "972500000000@c.us"},
  "timestamp": 1750000000,
  "idMessage": "BAE587FA1CECF760",
  "senderData": {
    "chatId": "120363001234567890@g.us",
    "sender": "972501234567@c.us",
    "senderName": "Dana"
  },
  "messageData": {
    "typeMessage": "textMessage",
    "textMessageData": {"textMessage": "מזל טוב"}
  }
}`

const gowaMessageBody = `{
  "event": "message",
  "device_id": "device-1",
  "payload": {
    "id": "GOWA-1",
    "chat_id": "120363001234567890@g.us",
    "from": "972501234567@s.whatsapp.net in 120363001234567890@g.us",
    "pushname": "Dana",
    "timestamp": 1750000000,
    "message": {"text": "happy birthday"}
  }
}`

func TestGreenAPIWebhookDispatchesAMessage(t *testing.T) {
	handler := &recordingHandler{}
	ta := newTestAPI(t, func(d *Deps) { d.Incoming = handler })

	rec := ta.postRaw(t, "/webhooks/greenapi", greenAPIMessageBody, nil)
	requireStatus(t, rec, http.StatusOK)

	got := handler.messages()
	if len(got) != 1 {
		t.Fatalf("dispatched %d messages, want 1", len(got))
	}
	if !got[0].IsGroup || got[0].Text != "מזל טוב" {
		t.Fatalf("unexpected message: %#v", got[0])
	}
}

func TestGreenAPIWebhookRejectsWrongAuthHeader(t *testing.T) {
	handler := &recordingHandler{}
	ta := newTestAPI(t, func(d *Deps) { d.Incoming = handler })
	saveSettings(t, ta, func(s *settings.Settings) {
		s.GreenAPI.WebhookAuthHeader = "Bearer expected"
	})

	rec := ta.postRaw(t, "/webhooks/greenapi", greenAPIMessageBody,
		map[string]string{"Authorization": "Bearer wrong"})
	requireStatus(t, rec, http.StatusUnauthorized)
	if got := errorCode(t, rec); got != codeUnauthorized {
		t.Fatalf("code = %q, want %q", got, codeUnauthorized)
	}
	if len(handler.messages()) != 0 {
		t.Fatal("an unauthorized webhook must not reach the handler")
	}
}

func TestGreenAPIWebhookAcceptsCorrectAuthHeader(t *testing.T) {
	handler := &recordingHandler{}
	ta := newTestAPI(t, func(d *Deps) { d.Incoming = handler })
	saveSettings(t, ta, func(s *settings.Settings) {
		s.GreenAPI.WebhookAuthHeader = "Bearer expected"
	})

	rec := ta.postRaw(t, "/webhooks/greenapi", greenAPIMessageBody,
		map[string]string{"Authorization": "Bearer expected"})
	requireStatus(t, rec, http.StatusOK)
	if len(handler.messages()) != 1 {
		t.Fatal("want the authorized webhook dispatched")
	}
}

// Providers retry on non-2xx; an uninteresting notification is not an error.
func TestGreenAPIWebhookIgnoresUnhandledNotifications(t *testing.T) {
	handler := &recordingHandler{}
	ta := newTestAPI(t, func(d *Deps) { d.Incoming = handler })

	rec := ta.postRaw(t, "/webhooks/greenapi",
		`{"typeWebhook":"outgoingMessageStatus","timestamp":1750000000}`, nil)
	requireStatus(t, rec, http.StatusOK)
	if len(handler.messages()) != 0 {
		t.Fatal("a status notification must not be dispatched as a message")
	}
}

func TestGreenAPIWebhookRejectsMalformedBody(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.postRaw(t, "/webhooks/greenapi", `{"typeWebhook":`, nil)
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestGOWAWebhookRequiresAConfiguredSecret(t *testing.T) {
	handler := &recordingHandler{}
	ta := newTestAPI(t, func(d *Deps) { d.Incoming = handler })
	// Defaults leave the secret empty. Accepting unverified webhooks would let
	// anyone on the LAN inject fake wishes, so this must fail closed.

	rec := ta.postRaw(t, "/webhooks/gowa", gowaMessageBody, nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	if len(handler.messages()) != 0 {
		t.Fatal("an unverified webhook must not reach the handler")
	}
}

func TestGOWAWebhookRejectsBadSignature(t *testing.T) {
	handler := &recordingHandler{}
	ta := newTestAPI(t, func(d *Deps) { d.Incoming = handler })
	saveSettings(t, ta, func(s *settings.Settings) { s.GOWA.WebhookSecret = "topsecret" })

	rec := ta.postRaw(t, "/webhooks/gowa", gowaMessageBody, map[string]string{
		gowa.SignatureHeader: signGOWA("wrongsecret", gowaMessageBody),
	})
	requireStatus(t, rec, http.StatusUnauthorized)
	if len(handler.messages()) != 0 {
		t.Fatal("a bad signature must not reach the handler")
	}
}

func TestGOWAWebhookAcceptsValidSignature(t *testing.T) {
	handler := &recordingHandler{}
	ta := newTestAPI(t, func(d *Deps) { d.Incoming = handler })
	saveSettings(t, ta, func(s *settings.Settings) { s.GOWA.WebhookSecret = "topsecret" })

	rec := ta.postRaw(t, "/webhooks/gowa", gowaMessageBody, map[string]string{
		gowa.SignatureHeader: signGOWA("topsecret", gowaMessageBody),
	})
	requireStatus(t, rec, http.StatusOK)

	got := handler.messages()
	if len(got) != 1 {
		t.Fatalf("dispatched %d messages, want 1", len(got))
	}
	if got[0].Text != "happy birthday" || !got[0].IsGroup {
		t.Fatalf("unexpected message: %#v", got[0])
	}
	if got[0].Provider != gowa.Name {
		t.Errorf("provider = %q, want %q", got[0].Provider, gowa.Name)
	}
}

// The signature covers the exact bytes; the handler must verify what it read.
func TestGOWAWebhookRejectsTamperedBody(t *testing.T) {
	handler := &recordingHandler{}
	ta := newTestAPI(t, func(d *Deps) { d.Incoming = handler })
	saveSettings(t, ta, func(s *settings.Settings) { s.GOWA.WebhookSecret = "topsecret" })

	signature := signGOWA("topsecret", gowaMessageBody)
	tampered := strings.Replace(gowaMessageBody, "happy birthday", "happy birthdays", 1)

	rec := ta.postRaw(t, "/webhooks/gowa", tampered, map[string]string{gowa.SignatureHeader: signature})
	requireStatus(t, rec, http.StatusUnauthorized)
	if len(handler.messages()) != 0 {
		t.Fatal("a tampered body must not reach the handler")
	}
}

// The message was authentic and delivered; a downstream failure is ours to fix,
// not something the provider should retry.
func TestWebhookHandlerFailureStillReturns200(t *testing.T) {
	ta := newTestAPI(t, func(d *Deps) {
		d.Incoming = &recordingHandler{err: errors.New("echo engine exploded")}
	})

	rec := ta.postRaw(t, "/webhooks/greenapi", greenAPIMessageBody, nil)
	requireStatus(t, rec, http.StatusOK)
}

// Before Phase 6 there is no consumer; the endpoint must still accept and log.
func TestWebhookWithoutHandlerStillAccepts(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.postRaw(t, "/webhooks/greenapi", greenAPIMessageBody, nil)
	requireStatus(t, rec, http.StatusOK)
}
