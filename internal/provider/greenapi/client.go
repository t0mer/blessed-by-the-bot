// Package greenapi implements the WhatsApp provider backed by GreenAPI's cloud
// REST service.
//
// Every endpoint follows the documented shape
// {apiUrl}/waInstance{idInstance}/{method}/{apiToken}. The API token is a path
// segment, so no code here may log or embed a full request URL — the shared
// transport reduces URLs to scheme://host for exactly this reason.
package greenapi

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

// Name is the provider's identifier in settings and the send log.
const Name = "greenapi"

// DefaultAPIURL is GreenAPI's shared endpoint. Instances assigned to a cluster
// use a host such as https://7103.api.greenapi.com instead.
const DefaultAPIURL = "https://api.green-api.com"

// Instance states reported by getStateInstance.
const (
	StateAuthorized    = "authorized"
	StateNotAuthorized = "notAuthorized"
)

// Config is the GreenAPI half of the application settings.
type Config struct {
	APIURL            string
	IDInstance        string
	APIToken          string
	Mode              string
	WebhookAuthHeader string
}

// Client talks to one GreenAPI instance.
type Client struct {
	cfg    Config
	apiURL string
	tr     *provider.Transport
	log    *slog.Logger
}

// New validates cfg and returns a ready client.
func New(cfg Config, tr *provider.Transport, log *slog.Logger) (*Client, error) {
	if strings.TrimSpace(cfg.IDInstance) == "" {
		return nil, errors.New("greenapi: instance id is required")
	}
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, errors.New("greenapi: api token is required")
	}
	if tr == nil {
		return nil, errors.New("greenapi: transport is required")
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	apiURL := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}

	// Whitespace in these fields corrupts the URL path and produces opaque 400s,
	// so trim once here rather than at every call site.
	cfg.IDInstance = strings.TrimSpace(cfg.IDInstance)
	cfg.APIToken = strings.TrimSpace(cfg.APIToken)

	return &Client{cfg: cfg, apiURL: apiURL, tr: tr, log: log}, nil
}

// Name identifies the provider.
func (c *Client) Name() string { return Name }

// APIURL reports the resolved base URL, for diagnostics and tests.
func (c *Client) APIURL() string { return c.apiURL }

// Mode reports the configured incoming-message mode.
func (c *Client) Mode() string { return c.cfg.Mode }

// endpoint builds a method URL. The result embeds the API token — never log it.
func (c *Client) endpoint(method string) string {
	return fmt.Sprintf("%s/waInstance%s/%s/%s", c.apiURL, c.cfg.IDInstance, method, c.cfg.APIToken)
}

type sendMessageRequest struct {
	ChatID  string `json:"chatId"`
	Message string `json:"message"`
}

type sendMessageResponse struct {
	IDMessage string `json:"idMessage"`
}

// SendText sends a text message and returns the provider's message id.
func (c *Client) SendText(ctx context.Context, chatID, text string) (string, error) {
	normalized, err := provider.NormalizeChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("greenapi: %w", err)
	}

	var resp sendMessageResponse
	if err := c.tr.Do(ctx, "POST", c.endpoint("sendMessage"), nil,
		sendMessageRequest{ChatID: normalized, Message: text}, &resp); err != nil {
		return "", c.redact(fmt.Errorf("greenapi: sending message: %w", err))
	}
	return resp.IDMessage, nil
}

type contact struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ListGroups returns the group chats visible to this instance, for the
// group-picker UI. GreenAPI exposes them through getContacts, tagged type "group".
func (c *Client) ListGroups(ctx context.Context) ([]provider.Group, error) {
	var contacts []contact
	if err := c.tr.Do(ctx, "GET", c.endpoint("getContacts"), nil, nil, &contacts); err != nil {
		return nil, c.redact(fmt.Errorf("greenapi: listing contacts: %w", err))
	}

	groups := make([]provider.Group, 0, len(contacts))
	for _, item := range contacts {
		if item.Type != "group" && !provider.IsGroupChatID(item.ID) {
			continue
		}
		groups = append(groups, provider.Group{ChatID: item.ID, Name: item.Name})
	}
	return groups, nil
}

type stateResponse struct {
	StateInstance string `json:"stateInstance"`
}

// Status reports the instance's authorisation state.
func (c *Client) Status(ctx context.Context) (provider.ProviderStatus, error) {
	var resp stateResponse
	if err := c.tr.Do(ctx, "GET", c.endpoint("getStateInstance"), nil, nil, &resp); err != nil {
		return provider.ProviderStatus{Provider: Name}, c.redact(fmt.Errorf("greenapi: reading state: %w", err))
	}

	status := provider.ProviderStatus{
		Provider:  Name,
		State:     resp.StateInstance,
		Connected: resp.StateInstance == StateAuthorized,
		NeedsQR:   resp.StateInstance == StateNotAuthorized,
	}
	switch resp.StateInstance {
	case StateNotAuthorized:
		status.Detail = "Scan the QR code in the GreenAPI console to authorise this instance."
	case "blocked":
		status.Detail = "The GreenAPI instance is blocked; check your account there."
	case "sleepMode":
		status.Detail = "The instance is in sleep mode and will not deliver messages."
	case "starting":
		status.Detail = "The instance is still starting; try again shortly."
	}
	return status, nil
}

// redactedError hides the API token in an error message while keeping the
// wrapped error inspectable with errors.As and errors.Is.
type redactedError struct {
	err   error
	token string
}

func (e *redactedError) Error() string {
	return strings.ReplaceAll(e.err.Error(), e.token, "***")
}

func (e *redactedError) Unwrap() error { return e.err }

// redact scrubs the API token from an error.
//
// The shared transport already reduces URLs to scheme://host, but a provider is
// free to echo the request URL — token path segment and all — back inside an
// error response body, which the transport quotes verbatim. This closes that
// path so a 401 can never put the credential into a log line.
func (c *Client) redact(err error) error {
	if err == nil || c.cfg.APIToken == "" {
		return err
	}
	return &redactedError{err: err, token: c.cfg.APIToken}
}

// CheckAuthHeader compares a configured webhook token against the value a
// request carried, in constant time. An empty expected token accepts anything,
// which matches GreenAPI's own default of an unauthenticated webhook.
//
// It is a package-level function because the webhook handler verifies against
// the token in settings, and no client instance exists when another provider is
// the active one.
func CheckAuthHeader(expected, received string) error {
	want := strings.TrimSpace(expected)
	if want == "" {
		return nil
	}
	if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(received))) {
		return errors.New("greenapi: webhook authorization header does not match")
	}
	return nil
}

// CheckAuthHeader verifies received against this client's configured token.
func (c *Client) CheckAuthHeader(received string) error {
	return CheckAuthHeader(c.cfg.WebhookAuthHeader, received)
}
