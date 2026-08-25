package handlers

import (
	"net/http"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

// defaultTestMessage is what "Send test message" sends when the user does not
// type their own. It names the app so the recipient knows why their phone buzzed.
const defaultTestMessage = "Test message from blessed-by-the-bot ✅"

// providerTestRequest asks the active provider to send one real message.
// Phone accepts either a bare number or a full chat id, so the same button can
// verify a group.
type providerTestRequest struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

type providerTestResponse struct {
	Provider  string `json:"provider"`
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
}

func (a *API) providerStatus(w http.ResponseWriter, r *http.Request) {
	active, err := a.providers.Active()
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	status, err := active.Status(r.Context())
	if err != nil {
		a.log.Warn("provider status check failed", "provider", active.Name(), "error", err)
		writeError(w, a.log, errorf(http.StatusBadGateway, codeProviderFailed,
			"the whatsapp provider is unreachable: %s", err))
		return
	}
	WriteJSON(w, http.StatusOK, status)
}

// providerTest sends one real WhatsApp message so the user can confirm their
// credentials. It goes through the active provider, which the factory wraps in
// the outbound rate limiter, so this cannot be used to bypass send pacing.
func (a *API) providerTest(w http.ResponseWriter, r *http.Request) {
	var req providerTestRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, a.log, err)
		return
	}

	// NormalizeChatID passes a full JID through untouched and turns a bare
	// number into <digits>@c.us, so the same field verifies a group or a person.
	chatID, err := provider.NormalizeChatID(req.Phone)
	if err != nil {
		v := newValidator()
		v.add("phone", "must be an international number or a full WhatsApp chat id")
		writeError(w, a.log, v.err())
		return
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		message = defaultTestMessage
	}

	active, err := a.providers.Active()
	if err != nil {
		writeError(w, a.log, err)
		return
	}

	messageID, err := active.SendText(r.Context(), chatID, message)
	if err != nil {
		a.log.Warn("provider test send failed", "provider", active.Name(), "error", err)
		writeError(w, a.log, errorf(http.StatusBadGateway, codeProviderFailed,
			"sending the test message failed: %s", err))
		return
	}

	a.log.Info("provider test message sent", "provider", active.Name(), "chat_id", chatID)
	WriteJSON(w, http.StatusOK, providerTestResponse{
		Provider:  active.Name(),
		ChatID:    chatID,
		MessageID: messageID,
	})
}
