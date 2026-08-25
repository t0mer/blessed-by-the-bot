// Package provider defines the WhatsApp provider abstraction: one interface both
// backends implement, the normalized incoming-message type, the shared HTTP
// transport, and a manager that swaps the active provider at runtime.
//
// This package is a leaf: the concrete greenapi and gowa packages import it, so
// it must never import them. Construction lives in provider/factory.
package provider

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrNoProvider is returned by Manager.Active before any provider is configured.
var ErrNoProvider = errors.New("no whatsapp provider is configured")

// Provider is the outbound surface every WhatsApp backend implements. Incoming
// messages deliberately do not go through it — each backend parses its own
// webhook or poll payload into an IncomingMessage instead.
type Provider interface {
	Name() string
	SendText(ctx context.Context, chatID, text string) (messageID string, err error)
	ListGroups(ctx context.Context) ([]Group, error)
	Status(ctx context.Context) (ProviderStatus, error)
}

// Group is a WhatsApp group as offered to the group-picker UI.
type Group struct {
	ChatID string `json:"chat_id"`
	Name   string `json:"name"`
}

// ProviderStatus reports whether the backend is usable right now.
//
// The name stutters as provider.ProviderStatus, which revive flags. It is kept
// because CLAUDE.md §4 names this type explicitly in the interface contract;
// renaming it would put the code out of step with the spec every later phase
// is written against.
//
//nolint:revive // spec-mandated type name
type ProviderStatus struct {
	Provider  string `json:"provider"`
	Connected bool   `json:"connected"`
	State     string `json:"state"`
	NeedsQR   bool   `json:"needs_qr"`
	Detail    string `json:"detail,omitempty"`
}

// IncomingMessage is the single internal shape both providers normalize into.
type IncomingMessage struct {
	Provider   string
	ChatID     string
	IsGroup    bool
	SenderID   string
	SenderName string
	Text       string
	Timestamp  time.Time
	MessageID  string
}

// Manager holds the active provider behind a mutex so a settings change can
// replace it without restarting the process (spec §4).
type Manager struct {
	mu     sync.RWMutex
	active Provider
}

// NewManager returns a Manager with no provider configured.
func NewManager() *Manager { return &Manager{} }

// Set installs p as the active provider, replacing any previous one.
func (m *Manager) Set(p Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = p
}

// Clear removes the active provider.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = nil
}

// Active returns the current provider, or ErrNoProvider when none is configured.
func (m *Manager) Active() (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.active == nil {
		return nil, ErrNoProvider
	}
	return m.active, nil
}

// Name reports the active provider's name, or "" when none is configured.
func (m *Manager) Name() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.active == nil {
		return ""
	}
	return m.active.Name()
}
