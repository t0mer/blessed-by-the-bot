package gowa_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider/gowa"
)

const secret = "webhook-secret"

func sign(t *testing.T, key string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignatureAcceptsValidPrefixedDigest(t *testing.T) {
	body := []byte(`{"event":"message"}`)
	if err := gowa.VerifySignature(secret, body, "sha256="+sign(t, secret, body)); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifySignatureAcceptsBareHex(t *testing.T) {
	body := []byte(`{"event":"message"}`)
	if err := gowa.VerifySignature(secret, body, sign(t, secret, body)); err != nil {
		t.Fatalf("valid bare-hex signature rejected: %v", err)
	}
}

func TestVerifySignatureIsCaseInsensitiveOnHex(t *testing.T) {
	body := []byte(`{"event":"message"}`)
	upper := strings.ToUpper(sign(t, secret, body))
	if err := gowa.VerifySignature(secret, body, "sha256="+upper); err != nil {
		t.Fatalf("upper-case hex rejected: %v", err)
	}
}

func TestVerifySignatureRejectsWrongSecret(t *testing.T) {
	body := []byte(`{"event":"message"}`)
	if err := gowa.VerifySignature(secret, body, sign(t, "other-secret", body)); err == nil {
		t.Fatal("signature made with the wrong secret accepted")
	}
}

func TestVerifySignatureRejectsTamperedBody(t *testing.T) {
	body := []byte(`{"event":"message","text":"hello"}`)
	signature := sign(t, secret, body)

	tampered := make([]byte, len(body))
	copy(tampered, body)
	tampered[len(tampered)-3] ^= 0xff

	if err := gowa.VerifySignature(secret, tampered, signature); err == nil {
		t.Fatal("tampered body accepted")
	}
}

func TestVerifySignatureRejectsEmptySecret(t *testing.T) {
	body := []byte(`{"event":"message"}`)
	// Accepting everything when misconfigured would let anyone forge group
	// traffic and trigger the bot; refusing is the only safe default.
	if err := gowa.VerifySignature("", body, sign(t, "", body)); err == nil {
		t.Fatal("empty secret accepted; unauthenticated webhooks must be refused")
	}
}

func TestVerifySignatureRejectsMissingOrGarbageHeader(t *testing.T) {
	body := []byte(`{"event":"message"}`)
	for _, received := range []string{"", "sha256=", "not-hex", "sha256=zzzz", "sha1=abcd"} {
		if err := gowa.VerifySignature(secret, body, received); err == nil {
			t.Errorf("header %q accepted", received)
		}
	}
}

const messagePayload = `{
  "event": "message",
  "device_id": "device-7",
  "payload": {
    "id": "3EB0ABC",
    "chat_id": "972501234567@c.us",
    "from": "972501234567@c.us",
    "pushname": "Dana",
    "message": {"text": "מזל טוב"},
    "timestamp": 1588091580
  }
}`

func TestParseWebhookMessageEvent(t *testing.T) {
	msg, handled, err := gowa.ParseWebhook([]byte(messagePayload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !handled {
		t.Fatal("handled = false for a message event")
	}
	if msg.Provider != "gowa" {
		t.Errorf("provider = %q", msg.Provider)
	}
	if msg.ChatID != "972501234567@c.us" {
		t.Errorf("chat id = %q", msg.ChatID)
	}
	if msg.SenderID != "972501234567@c.us" || msg.SenderName != "Dana" {
		t.Errorf("sender = %q/%q", msg.SenderID, msg.SenderName)
	}
	if msg.Text != "מזל טוב" {
		t.Errorf("text = %q", msg.Text)
	}
	if msg.MessageID != "3EB0ABC" {
		t.Errorf("message id = %q", msg.MessageID)
	}
	if want := time.Unix(1588091580, 0).UTC(); !msg.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", msg.Timestamp, want)
	}
	if msg.IsGroup {
		t.Error("private chat reported as a group")
	}
}

func TestParseWebhookGroupMessage(t *testing.T) {
	payload := `{
	  "event": "message",
	  "payload": {
	    "id": "G1",
	    "chat_id": "120363012345678901@g.us",
	    "from": "972501234567@c.us",
	    "pushname": "Dana",
	    "message": {"text": "מזל טוב"},
	    "timestamp": 1588091580
	  }
	}`
	msg, handled, err := gowa.ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !handled {
		t.Fatal("handled = false")
	}
	if !msg.IsGroup {
		t.Error("group message not flagged as a group")
	}
	if msg.SenderID != "972501234567@c.us" {
		t.Errorf("sender id = %q, want the participant not the group", msg.SenderID)
	}
}

func TestParseWebhookHandlesCompositeFromField(t *testing.T) {
	// GOWA sometimes reports group senders as "participant in group".
	payload := `{
	  "event": "message",
	  "payload": {
	    "id": "G2",
	    "chat_id": "120363012345678901@g.us",
	    "from": "972501234567@s.whatsapp.net in 120363012345678901@g.us",
	    "message": {"text": "מזל טוב"},
	    "timestamp": 1588091580
	  }
	}`
	msg, handled, err := gowa.ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !handled {
		t.Fatal("handled = false")
	}
	if msg.SenderID != "972501234567@s.whatsapp.net" {
		t.Errorf("sender id = %q, want just the participant", msg.SenderID)
	}
}

func TestParseWebhookIgnoresNonMessageEvents(t *testing.T) {
	for _, event := range []string{"message.ack", "qr", "ready"} {
		payload := `{"event":"` + event + `","payload":{}}`
		_, handled, err := gowa.ParseWebhook([]byte(payload))
		if err != nil {
			t.Errorf("%s: unexpected error %v", event, err)
		}
		if handled {
			t.Errorf("%s: handled = true, want false", event)
		}
	}
}

func TestParseWebhookIgnoresEmptyText(t *testing.T) {
	payload := `{"event":"message","payload":{"id":"x","chat_id":"a@c.us","message":{}}}`
	_, handled, err := gowa.ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if handled {
		t.Error("a message with no text was handled")
	}
}

func TestParseWebhookRejectsMalformedJSON(t *testing.T) {
	if _, _, err := gowa.ParseWebhook([]byte(`{not json`)); err == nil {
		t.Fatal("malformed JSON accepted")
	}
}
