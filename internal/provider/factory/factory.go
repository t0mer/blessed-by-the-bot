// Package factory builds a concrete WhatsApp provider from application settings.
//
// It exists to keep internal/provider a leaf package: greenapi and gowa import
// provider, so provider cannot import them back. Everything that needs to know
// about both concrete backends lives here instead.
package factory

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/gowa"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/greenapi"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
)

// Build constructs the provider selected in s, wrapped in the outbound rate
// limiter so every send is paced regardless of which backend is active.
func Build(s *settings.Settings, tr *provider.Transport, log *slog.Logger) (provider.Provider, error) {
	if s == nil {
		return nil, errors.New("provider factory: settings are required")
	}

	var (
		built provider.Provider
		err   error
	)
	switch s.Provider {
	case settings.ProviderGreenAPI:
		built, err = greenapi.New(greenapi.Config{
			APIURL:            s.GreenAPI.APIURL,
			IDInstance:        s.GreenAPI.IDInstance,
			APIToken:          s.GreenAPI.APIToken,
			Mode:              s.GreenAPI.Mode,
			WebhookAuthHeader: s.GreenAPI.WebhookAuthHeader,
		}, tr, log)
	case settings.ProviderGOWA:
		built, err = gowa.New(gowa.Config{
			BaseURL:       s.GOWA.BaseURL,
			Username:      s.GOWA.Username,
			Password:      s.GOWA.Password,
			DeviceID:      s.GOWA.DeviceID,
			WebhookSecret: s.GOWA.WebhookSecret,
		}, tr, log)
	default:
		return nil, fmt.Errorf("provider factory: unknown provider %q", s.Provider)
	}
	if err != nil {
		return nil, err
	}

	return provider.RateLimited(built, provider.DefaultSendInterval), nil
}

// Rebuild builds the configured provider and installs it as the active one, so a
// settings change takes effect without restarting the process (spec §4).
//
// On failure the manager keeps whatever provider it already had: dropping a
// working connection because of a bad edit would be worse than rejecting the edit.
func Rebuild(mgr *provider.Manager, s *settings.Settings, tr *provider.Transport, log *slog.Logger) error {
	built, err := Build(s, tr, log)
	if err != nil {
		return err
	}
	mgr.Set(built)
	return nil
}
