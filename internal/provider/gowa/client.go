// Package gowa implements the WhatsApp provider backed by a self-hosted
// go-whatsapp-web-multidevice (GOWA) instance, targeting v8.
//
// v8 scopes device calls by an X-Device-Id header. When no device is configured
// the header is omitted entirely so GOWA's single-device fallback applies —
// sending an empty header would not trigger that fallback.
//
// Response shapes are parsed tolerantly: GOWA has moved field names between
// releases, and a send that actually succeeded must not be reported as failed
// because an id field was renamed. See docs/providers.md for the assumptions.
package gowa

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

// Name is the provider's identifier in settings and the send log.
const Name = "gowa"

// DeviceHeader scopes a request to one registered device (GOWA v8).
const DeviceHeader = "X-Device-Id"

// Config is the GOWA half of the application settings.
type Config struct {
	BaseURL       string
	Username      string
	Password      string
	DeviceID      string
	WebhookSecret string
}

// Client talks to one GOWA instance.
type Client struct {
	cfg     Config
	baseURL string
	tr      *provider.Transport
	log     *slog.Logger
}

// New validates cfg and returns a ready client.
func New(cfg Config, tr *provider.Transport, log *slog.Logger) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("gowa: base url is required")
	}
	if tr == nil {
		return nil, errors.New("gowa: transport is required")
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	// Whitespace in credentials corrupts the Authorization header and produces
	// opaque 400s, so trim once here.
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.Password = strings.TrimSpace(cfg.Password)
	cfg.DeviceID = strings.TrimSpace(cfg.DeviceID)

	return &Client{cfg: cfg, baseURL: baseURL, tr: tr, log: log}, nil
}

// Name identifies the provider.
func (c *Client) Name() string { return Name }

// BaseURL reports the resolved base URL, for diagnostics and tests.
func (c *Client) BaseURL() string { return c.baseURL }

// WebhookSecret returns the configured HMAC secret for webhook verification.
func (c *Client) WebhookSecret() string { return c.cfg.WebhookSecret }

// DeviceID returns the configured device scope, empty when unset.
func (c *Client) DeviceID() string { return c.cfg.DeviceID }

// headers builds the auth and device-scoping headers for a request.
func (c *Client) headers() http.Header {
	h := http.Header{}
	if c.cfg.Username != "" || c.cfg.Password != "" {
		credentials := base64.StdEncoding.EncodeToString([]byte(c.cfg.Username + ":" + c.cfg.Password))
		h.Set("Authorization", "Basic "+credentials)
	}
	if c.cfg.DeviceID != "" {
		h.Set(DeviceHeader, c.cfg.DeviceID)
	}
	return h
}

type sendMessageRequest struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

// sendMessageResponse accepts the documented key plus the aliases GOWA has used
// across releases.
type sendMessageResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Results struct {
		MessageID    string `json:"message_id"`
		MessageIDAlt string `json:"messageId"`
		ID           string `json:"id"`
		Status       string `json:"status"`
	} `json:"results"`
}

func (r *sendMessageResponse) messageID() string {
	for _, candidate := range []string{r.Results.MessageID, r.Results.MessageIDAlt, r.Results.ID} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// SendText sends a text message and returns GOWA's message id when it supplies one.
func (c *Client) SendText(ctx context.Context, chatID, text string) (string, error) {
	normalized, err := provider.NormalizeChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("gowa: %w", err)
	}

	var resp sendMessageResponse
	if err := c.tr.Do(ctx, http.MethodPost, c.baseURL+"/send/message", c.headers(),
		sendMessageRequest{Phone: normalized, Message: text}, &resp); err != nil {
		return "", fmt.Errorf("gowa: sending message: %w", err)
	}
	return resp.messageID(), nil
}

type groupsResponse struct {
	Results struct {
		Data []struct {
			JID     string `json:"JID"`
			JIDAlt  string `json:"jid"`
			Name    string `json:"Name"`
			NameAlt string `json:"name"`
		} `json:"data"`
	} `json:"results"`
}

// ListGroups returns the groups this device belongs to, for the group-picker UI.
func (c *Client) ListGroups(ctx context.Context) ([]provider.Group, error) {
	var resp groupsResponse
	if err := c.tr.Do(ctx, http.MethodGet, c.baseURL+"/user/my/groups", c.headers(), nil, &resp); err != nil {
		return nil, fmt.Errorf("gowa: listing groups: %w", err)
	}

	groups := make([]provider.Group, 0, len(resp.Results.Data))
	for _, item := range resp.Results.Data {
		chatID := item.JID
		if chatID == "" {
			chatID = item.JIDAlt
		}
		if chatID == "" {
			continue
		}
		name := item.Name
		if name == "" {
			name = item.NameAlt
		}
		groups = append(groups, provider.Group{ChatID: chatID, Name: name})
	}
	return groups, nil
}

type statusResponse struct {
	Results struct {
		IsConnected bool   `json:"is_connected"`
		IsLoggedIn  bool   `json:"is_logged_in"`
		DeviceID    string `json:"device_id"`
	} `json:"results"`
}

// Status reports whether the GOWA device is connected and logged in.
func (c *Client) Status(ctx context.Context) (provider.ProviderStatus, error) {
	var resp statusResponse
	if err := c.tr.Do(ctx, http.MethodGet, c.baseURL+"/app/status", c.headers(), nil, &resp); err != nil {
		return provider.ProviderStatus{Provider: Name}, fmt.Errorf("gowa: reading status: %w", err)
	}

	status := provider.ProviderStatus{
		Provider:  Name,
		Connected: resp.Results.IsConnected && resp.Results.IsLoggedIn,
	}
	switch {
	case !resp.Results.IsConnected:
		status.State = "disconnected"
		status.Detail = "GOWA is not connected to WhatsApp. Check that the container is running and reachable at " + c.baseURL + "."
	case !resp.Results.IsLoggedIn:
		status.State = "logged_out"
		status.NeedsQR = true
		// v1 deliberately does not proxy the QR flow, so point the user at GOWA.
		status.Detail = "Scan the QR code in the GOWA web UI at " + c.baseURL + " to log this device in."
	default:
		status.State = "connected"
	}
	return status, nil
}
