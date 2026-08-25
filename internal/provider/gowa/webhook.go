package gowa

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

// SignatureHeader carries the HMAC-SHA256 digest GOWA signs each payload with.
const SignatureHeader = "X-Hub-Signature-256"

// signaturePrefix is the optional algorithm marker on the header value.
const signaturePrefix = "sha256="

// eventMessage is the only event this application subscribes to
// (--webhook-events=message).
const eventMessage = "message"

// VerifySignature checks an HMAC-SHA256 digest over the raw request body.
//
// The comparison is constant time. An empty secret is rejected rather than
// treated as "no verification": an unauthenticated webhook would let anyone
// forge group traffic and make the bot post on demand.
func VerifySignature(secret string, body []byte, received string) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("gowa: webhook secret is not configured; refusing to accept unverified webhooks")
	}

	candidate := strings.ToLower(strings.TrimSpace(received))
	candidate = strings.TrimPrefix(candidate, signaturePrefix)
	if candidate == "" {
		return errors.New("gowa: webhook signature header is missing")
	}

	// Decode first so a malformed header fails cleanly instead of comparing
	// mismatched lengths.
	got, err := hex.DecodeString(candidate)
	if err != nil {
		return fmt.Errorf("gowa: webhook signature is not valid hex: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return errors.New("gowa: webhook signature does not match")
	}
	return nil
}

// event mirrors a GOWA webhook payload. Field aliases are accepted because GOWA
// has renamed several of these between releases.
type event struct {
	Event    string `json:"event"`
	DeviceID string `json:"device_id"`
	Payload  struct {
		ID        string `json:"id"`
		ChatID    string `json:"chat_id"`
		From      string `json:"from"`
		PushName  string `json:"pushname"`
		Timestamp int64  `json:"timestamp"`
		Message   struct {
			Text         string `json:"text"`
			Conversation string `json:"conversation"`
		} `json:"message"`
		Text string `json:"text"`
	} `json:"payload"`
}

// ParseWebhook decodes a GOWA event into the shared incoming-message shape.
// handled is false for events this application ignores, which is not an error.
func ParseWebhook(body []byte) (*provider.IncomingMessage, bool, error) {
	var e event
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, false, fmt.Errorf("gowa: decoding webhook: %w", err)
	}
	if e.Event != eventMessage {
		return nil, false, nil
	}

	text := firstNonEmpty(e.Payload.Message.Text, e.Payload.Message.Conversation, e.Payload.Text)
	if text == "" {
		return nil, false, nil
	}

	sender := participantOf(e.Payload.From)
	if sender == "" {
		sender = e.Payload.ChatID
	}

	return &provider.IncomingMessage{
		Provider:   Name,
		ChatID:     e.Payload.ChatID,
		IsGroup:    provider.IsGroupChatID(e.Payload.ChatID),
		SenderID:   sender,
		SenderName: e.Payload.PushName,
		Text:       text,
		Timestamp:  time.Unix(e.Payload.Timestamp, 0).UTC(),
		MessageID:  e.Payload.ID,
	}, true, nil
}

// participantOf extracts the sender from GOWA's "from" field, which for group
// messages can read "<participant> in <group>". Distinct-sender counting breaks
// if the group id ends up here instead of the participant.
func participantOf(from string) string {
	trimmed := strings.TrimSpace(from)
	if idx := strings.Index(trimmed, " in "); idx >= 0 {
		return strings.TrimSpace(trimmed[:idx])
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
