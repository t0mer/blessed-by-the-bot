package greenapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/greenapi"
)

const textMessagePayload = `{
  "typeWebhook": "incomingMessageReceived",
  "instanceData": {"idInstance": 7103123456, "wid": "972500000000@c.us"},
  "timestamp": 1588091580,
  "idMessage": "BAE587FA1CECF760",
  "senderData": {
    "chatId": "972501234567@c.us",
    "sender": "972501234567@c.us",
    "senderName": "Dana"
  },
  "messageData": {
    "typeMessage": "textMessage",
    "textMessageData": {"textMessage": "מזל טוב"}
  }
}`

func TestParseWebhookTextMessage(t *testing.T) {
	msg, handled, err := greenapi.ParseWebhook([]byte(textMessagePayload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !handled {
		t.Fatal("handled = false for an incoming text message")
	}
	if msg.Provider != "greenapi" {
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
	if msg.MessageID != "BAE587FA1CECF760" {
		t.Errorf("message id = %q", msg.MessageID)
	}
	if want := time.Unix(1588091580, 0).UTC(); !msg.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", msg.Timestamp, want)
	}
	if msg.IsGroup {
		t.Error("private chat reported as a group")
	}
}

func TestParseWebhookExtendedTextMessage(t *testing.T) {
	payload := `{
	  "typeWebhook": "incomingMessageReceived",
	  "timestamp": 1588091580,
	  "idMessage": "X1",
	  "senderData": {"chatId": "972501234567@c.us", "sender": "972501234567@c.us", "senderName": "Dana"},
	  "messageData": {
	    "typeMessage": "extendedTextMessage",
	    "extendedTextMessageData": {"text": "happy birthday!"}
	  }
	}`
	msg, handled, err := greenapi.ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !handled {
		t.Fatal("handled = false for an extendedTextMessage")
	}
	if msg.Text != "happy birthday!" {
		t.Errorf("text = %q", msg.Text)
	}
}

func TestParseWebhookGroupMessageSetsIsGroup(t *testing.T) {
	payload := `{
	  "typeWebhook": "incomingMessageReceived",
	  "timestamp": 1588091580,
	  "idMessage": "G1",
	  "senderData": {
	    "chatId": "120363012345678901@g.us",
	    "sender": "972501234567@c.us",
	    "senderName": "Dana",
	    "chatName": "Family"
	  },
	  "messageData": {"typeMessage": "textMessage", "textMessageData": {"textMessage": "מזל טוב"}}
	}`
	msg, handled, err := greenapi.ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !handled {
		t.Fatal("handled = false")
	}
	if !msg.IsGroup {
		t.Error("group message not flagged as a group")
	}
	if msg.ChatID != "120363012345678901@g.us" {
		t.Errorf("chat id = %q", msg.ChatID)
	}
	// Counting distinct senders only works if this is the participant, not the group.
	if msg.SenderID != "972501234567@c.us" {
		t.Errorf("sender id = %q, want the participant", msg.SenderID)
	}
}

func TestParseWebhookIgnoresOtherTypes(t *testing.T) {
	for _, kind := range []string{"outgoingMessageStatus", "stateInstanceChanged", "outgoingAPIMessageReceived"} {
		payload := `{"typeWebhook":"` + kind + `","timestamp":1588091580}`
		msg, handled, err := greenapi.ParseWebhook([]byte(payload))
		if err != nil {
			t.Errorf("%s: unexpected error %v", kind, err)
		}
		if handled {
			t.Errorf("%s: handled = true, want false", kind)
		}
		if msg != nil {
			t.Errorf("%s: message = %+v, want nil", kind, msg)
		}
	}
}

func TestParseWebhookIgnoresNonTextMessageData(t *testing.T) {
	payload := `{
	  "typeWebhook": "incomingMessageReceived",
	  "timestamp": 1588091580,
	  "idMessage": "I1",
	  "senderData": {"chatId": "972501234567@c.us", "sender": "972501234567@c.us"},
	  "messageData": {"typeMessage": "imageMessage", "fileMessageData": {"downloadUrl": "http://x"}}
	}`
	_, handled, err := greenapi.ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if handled {
		t.Error("an image message was handled; only text carries wishes")
	}
}

func TestParseWebhookRejectsMalformedJSON(t *testing.T) {
	if _, _, err := greenapi.ParseWebhook([]byte(`{not json`)); err == nil {
		t.Fatal("malformed JSON accepted")
	}
}

func TestCheckAuthHeader(t *testing.T) {
	tr := provider.NewTransport(provider.TransportOptions{})

	withToken, err := greenapi.New(greenapi.Config{
		IDInstance: "1", APIToken: "t", WebhookAuthHeader: "shared-secret",
	}, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := withToken.CheckAuthHeader("shared-secret"); err != nil {
		t.Errorf("matching header rejected: %v", err)
	}
	if err := withToken.CheckAuthHeader("wrong"); err == nil {
		t.Error("mismatched header accepted")
	}
	if err := withToken.CheckAuthHeader(""); err == nil {
		t.Error("missing header accepted when a token is configured")
	}

	noToken, err := greenapi.New(greenapi.Config{IDInstance: "1", APIToken: "t"}, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := noToken.CheckAuthHeader("anything"); err != nil {
		t.Errorf("unconfigured webhook auth rejected a request: %v", err)
	}
}

var _ = http.MethodGet // keep the net/http import meaningful across edits
