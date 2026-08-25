// Package settings stores and retrieves runtime configuration, encrypting
// provider credentials at rest and masking them on the way out.
//
// Layering: this is the only package that decrypts settings secrets. Handlers
// receive masked values; the provider layer receives decrypted ones.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// Mask is what the API returns in place of a stored secret. A Save carrying
// this value for a secret field means "leave it unchanged".
const Mask = "••••"

// Provider names.
const (
	ProviderGreenAPI = "greenapi"
	ProviderGOWA     = "gowa"
)

// GreenAPI incoming-message modes.
const (
	ModePolling = "polling"
	ModeWebhook = "webhook"
)

// Settings keys in the store's key/value table.
const (
	keyProvider  = "provider"
	keyGreenAPI  = "provider.greenapi"
	keyGOWA      = "provider.gowa"
	keyScheduler = "scheduler"
	keyGroupEcho = "group_echo"
)

var sendTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// GreenAPIConfig is the GreenAPI provider configuration. APIToken and
// WebhookAuthHeader are secrets.
type GreenAPIConfig struct {
	APIURL            string `json:"api_url"`
	IDInstance        string `json:"id_instance"`
	APIToken          string `json:"api_token"`
	Mode              string `json:"mode"`
	WebhookAuthHeader string `json:"webhook_auth_header"`
}

// GOWAConfig is the go-whatsapp-web-multidevice configuration. Password and
// WebhookSecret are secrets.
type GOWAConfig struct {
	BaseURL       string `json:"base_url"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	DeviceID      string `json:"device_id"`
	WebhookSecret string `json:"webhook_secret"`
}

// SchedulerSettings covers when scheduled blessings go out.
type SchedulerSettings struct {
	Timezone string `json:"timezone"`
	SendTime string `json:"send_time"`
}

// GroupEchoSettings covers the group "join in" trigger.
type GroupEchoSettings struct {
	Threshold     int `json:"threshold"`
	WindowHours   int `json:"window_hours"`
	CooldownHours int `json:"cooldown_hours"`
}

// Settings is the full runtime configuration.
type Settings struct {
	Provider  string            `json:"provider"`
	GreenAPI  GreenAPIConfig    `json:"greenapi"`
	GOWA      GOWAConfig        `json:"gowa"`
	Scheduler SchedulerSettings `json:"scheduler"`
	GroupEcho GroupEchoSettings `json:"group_echo"`
}

// Defaults returns the settings a fresh installation starts with.
func Defaults() *Settings {
	return &Settings{
		Provider: ProviderGreenAPI,
		GreenAPI: GreenAPIConfig{
			APIURL: "https://api.green-api.com",
			Mode:   ModePolling,
		},
		Scheduler: SchedulerSettings{
			Timezone: "Asia/Jerusalem",
			SendTime: "09:00",
		},
		GroupEcho: GroupEchoSettings{
			Threshold:     3,
			WindowHours:   6,
			CooldownHours: 20,
		},
	}
}

// secretFields returns pointers to every field that must be encrypted at rest.
// Load, LoadMasked and Save all walk this one list, so adding a credential in
// future means editing exactly one place.
func (s *Settings) secretFields() []*string {
	return []*string{
		&s.GreenAPI.APIToken,
		&s.GreenAPI.WebhookAuthHeader,
		&s.GOWA.Password,
		&s.GOWA.WebhookSecret,
	}
}

func (s *Settings) validate() error {
	switch s.Provider {
	case ProviderGreenAPI, ProviderGOWA:
	default:
		return fmt.Errorf("unknown provider %q: want %s or %s",
			s.Provider, ProviderGreenAPI, ProviderGOWA)
	}
	switch s.GreenAPI.Mode {
	case "", ModePolling, ModeWebhook:
	default:
		return fmt.Errorf("unknown greenapi mode %q: want %s or %s",
			s.GreenAPI.Mode, ModePolling, ModeWebhook)
	}
	if _, err := time.LoadLocation(s.Scheduler.Timezone); err != nil {
		return fmt.Errorf("unknown timezone %q: %w", s.Scheduler.Timezone, err)
	}
	if !sendTimePattern.MatchString(s.Scheduler.SendTime) {
		return fmt.Errorf("send time %q must be HH:MM in 24-hour form", s.Scheduler.SendTime)
	}
	if s.GroupEcho.Threshold < 1 {
		return fmt.Errorf("group echo threshold %d must be at least 1", s.GroupEcho.Threshold)
	}
	if s.GroupEcho.WindowHours < 1 {
		return fmt.Errorf("group echo window %dh must be at least 1", s.GroupEcho.WindowHours)
	}
	if s.GroupEcho.CooldownHours < 1 {
		return fmt.Errorf("group echo cooldown %dh must be at least 1", s.GroupEcho.CooldownHours)
	}
	return nil
}

// Service reads and writes settings.
type Service struct {
	store  *store.Store
	cipher *crypto.Cipher
}

// New builds a Service. Both dependencies are required.
func New(st *store.Store, cipher *crypto.Cipher) (*Service, error) {
	if st == nil {
		return nil, errors.New("settings: store is required")
	}
	if cipher == nil {
		return nil, errors.New("settings: cipher is required")
	}
	return &Service{store: st, cipher: cipher}, nil
}

// Load returns settings with secrets decrypted, for internal consumers such as
// the provider manager and the scheduler.
func (s *Service) Load(ctx context.Context) (*Settings, error) {
	out := Defaults()

	provider, err := s.get(ctx, keyProvider)
	if err != nil {
		return nil, err
	}
	if provider != "" {
		out.Provider = provider
	}
	for key, dest := range map[string]any{
		keyGreenAPI:  &out.GreenAPI,
		keyGOWA:      &out.GOWA,
		keyScheduler: &out.Scheduler,
		keyGroupEcho: &out.GroupEcho,
	} {
		if err := s.getJSON(ctx, key, dest); err != nil {
			return nil, err
		}
	}

	for _, field := range out.secretFields() {
		plain, err := s.cipher.DecryptIfEncrypted(*field)
		if err != nil {
			return nil, fmt.Errorf("decrypting settings secret: %w", err)
		}
		*field = plain
	}
	return out, nil
}

// LoadMasked returns settings safe to hand to an API client: every secret that
// is set becomes Mask, and unset secrets stay empty so the UI can tell the
// difference between "configured" and "not configured".
func (s *Service) LoadMasked(ctx context.Context) (*Settings, error) {
	out, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}
	for _, field := range out.secretFields() {
		if *field != "" {
			*field = Mask
		}
	}
	return out, nil
}

// Save validates and persists in. A secret field equal to Mask keeps its stored
// value; an empty secret clears it.
func (s *Service) Save(ctx context.Context, in *Settings) error {
	if in == nil {
		return errors.New("settings: nothing to save")
	}
	next := *in

	current, err := s.Load(ctx)
	if err != nil {
		return err
	}
	nextSecrets := next.secretFields()
	currentSecrets := current.secretFields()
	for i := range nextSecrets {
		if *nextSecrets[i] == Mask {
			*nextSecrets[i] = *currentSecrets[i]
		}
	}

	if err := next.validate(); err != nil {
		return err
	}

	// Encrypt only after validation, so a rejected payload never reaches the store.
	for _, field := range next.secretFields() {
		if *field == "" {
			continue
		}
		enc, err := s.cipher.Encrypt(*field)
		if err != nil {
			return fmt.Errorf("encrypting settings secret: %w", err)
		}
		*field = enc
	}

	if err := s.store.SetSetting(ctx, keyProvider, next.Provider); err != nil {
		return err
	}
	for key, value := range map[string]any{
		keyGreenAPI:  next.GreenAPI,
		keyGOWA:      next.GOWA,
		keyScheduler: next.Scheduler,
		keyGroupEcho: next.GroupEcho,
	} {
		if err := s.setJSON(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) get(ctx context.Context, key string) (string, error) {
	v, err := s.store.GetSetting(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// getJSON decodes a stored JSON value into dest, leaving dest untouched when the
// key is absent so Defaults() survives.
func (s *Service) getJSON(ctx context.Context, key string, dest any) error {
	raw, err := s.get(ctx, key)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("decoding setting %q: %w", key, err)
	}
	return nil
}

func (s *Service) setJSON(ctx context.Context, key string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encoding setting %q: %w", key, err)
	}
	return s.store.SetSetting(ctx, key, string(body))
}
