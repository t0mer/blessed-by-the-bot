package handlers

import (
	"context"
	"io"
	"net/http"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/gowa"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/greenapi"
)

// greenAPIWebhook receives GreenAPI's incomingMessageReceived notifications.
//
// GreenAPI's webhook token, when configured, arrives as the Authorization
// header. An unset token accepts anything, which matches GreenAPI's own default
// and is why polling is the recommended mode for a LAN deployment.
func (a *API) greenAPIWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := readWebhookBody(w, r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}

	current, err := a.settings.Load(r.Context())
	if err != nil {
		writeError(w, a.log, err)
		return
	}

	if err := greenapi.CheckAuthHeader(current.GreenAPI.WebhookAuthHeader, r.Header.Get("Authorization")); err != nil {
		a.log.Warn("rejected greenapi webhook", "reason", "authorization header mismatch")
		writeError(w, a.log, errorf(http.StatusUnauthorized, codeUnauthorized,
			"webhook authorization failed"))
		return
	}

	msg, handled, err := greenapi.ParseWebhook(body)
	if err != nil {
		a.log.Warn("unparsable greenapi webhook", "error", err)
		writeError(w, a.log, errorf(http.StatusBadRequest, codeInvalidJSON,
			"webhook payload could not be parsed"))
		return
	}
	a.dispatch(r.Context(), greenapi.Name, msg, handled)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// gowaWebhook receives go-whatsapp-web-multidevice message events.
//
// Verification is mandatory: gowa.VerifySignature refuses to pass when no secret
// is configured, so a misconfigured install fails closed rather than accepting
// forged wishes from anything that can reach the port.
func (a *API) gowaWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := readWebhookBody(w, r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}

	current, err := a.settings.Load(r.Context())
	if err != nil {
		writeError(w, a.log, err)
		return
	}

	if err := gowa.VerifySignature(current.GOWA.WebhookSecret, body, r.Header.Get(gowa.SignatureHeader)); err != nil {
		a.log.Warn("rejected gowa webhook", "error", err)
		writeError(w, a.log, errorf(http.StatusUnauthorized, codeUnauthorized,
			"webhook signature verification failed"))
		return
	}

	msg, handled, err := gowa.ParseWebhook(body)
	if err != nil {
		a.log.Warn("unparsable gowa webhook", "error", err)
		writeError(w, a.log, errorf(http.StatusBadRequest, codeInvalidJSON,
			"webhook payload could not be parsed"))
		return
	}
	a.dispatch(r.Context(), gowa.Name, msg, handled)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// dispatch hands a parsed message to the incoming handler. Everything here is
// best-effort: the provider has already delivered the message, so neither an
// unhandled payload type nor a handler failure changes the HTTP response.
func (a *API) dispatch(ctx context.Context, name string, msg *provider.IncomingMessage, handled bool) {
	if !handled || msg == nil {
		a.log.Debug("webhook payload ignored", "provider", name)
		return
	}
	a.log.Info("inbound message",
		"provider", name, "chat_id", msg.ChatID, "is_group", msg.IsGroup, "sender", msg.SenderID)

	if a.incoming == nil {
		a.log.Debug("no incoming handler is registered; message dropped", "provider", name)
		return
	}
	if err := a.incoming.HandleIncoming(ctx, *msg); err != nil {
		a.log.Error("handling inbound message failed", "provider", name, "error", err)
	}
}

// readWebhookBody reads the whole body, which the GOWA HMAC needs before the
// payload can be parsed.
func readWebhookBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errorf(http.StatusRequestEntityTooLarge, codeInvalidJSON,
			"webhook body must not exceed %d bytes", maxBodyBytes)
	}
	return body, nil
}
