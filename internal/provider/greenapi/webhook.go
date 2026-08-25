package greenapi

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

// Notification types GreenAPI pushes. Only incoming messages matter here; the
// rest (delivery receipts, state changes, echoes of our own sends) are ignored.
const (
	typeIncomingMessage = "incomingMessageReceived"

	typeTextMessage         = "textMessage"
	typeExtendedTextMessage = "extendedTextMessage"
	typeQuotedMessage       = "quotedMessage"
)

// notification mirrors the documented incomingMessageReceived payload.
type notification struct {
	TypeWebhook string `json:"typeWebhook"`
	Timestamp   int64  `json:"timestamp"`
	IDMessage   string `json:"idMessage"`
	SenderData  struct {
		ChatID     string `json:"chatId"`
		Sender     string `json:"sender"`
		SenderName string `json:"senderName"`
		ChatName   string `json:"chatName"`
	} `json:"senderData"`
	MessageData struct {
		TypeMessage     string `json:"typeMessage"`
		TextMessageData struct {
			TextMessage string `json:"textMessage"`
		} `json:"textMessageData"`
		ExtendedTextMessageData struct {
			Text string `json:"text"`
		} `json:"extendedTextMessageData"`
	} `json:"messageData"`
}

// ParseWebhook decodes a GreenAPI notification into the shared incoming-message
// shape. handled is false for notification types this application ignores; that
// is not an error, and callers must still acknowledge such notifications.
func ParseWebhook(body []byte) (*provider.IncomingMessage, bool, error) {
	var n notification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, false, fmt.Errorf("greenapi: decoding webhook: %w", err)
	}
	return n.toMessage()
}

func (n *notification) toMessage() (*provider.IncomingMessage, bool, error) {
	if n.TypeWebhook != typeIncomingMessage {
		return nil, false, nil
	}

	var text string
	switch n.MessageData.TypeMessage {
	case typeTextMessage:
		text = n.MessageData.TextMessageData.TextMessage
	case typeExtendedTextMessage, typeQuotedMessage:
		text = n.MessageData.ExtendedTextMessageData.Text
	default:
		// Images, documents, stickers and the rest carry no wish text.
		return nil, false, nil
	}
	if text == "" {
		return nil, false, nil
	}

	sender := n.SenderData.Sender
	if sender == "" {
		// A private chat's sender is the chat itself; fall back so distinct-sender
		// counting never sees an empty key.
		sender = n.SenderData.ChatID
	}

	return &provider.IncomingMessage{
		Provider:   Name,
		ChatID:     n.SenderData.ChatID,
		IsGroup:    provider.IsGroupChatID(n.SenderData.ChatID),
		SenderID:   sender,
		SenderName: n.SenderData.SenderName,
		Text:       text,
		Timestamp:  time.Unix(n.Timestamp, 0).UTC(),
		MessageID:  n.IDMessage,
	}, true, nil
}
