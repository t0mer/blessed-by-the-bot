package factory_test

import (
	"testing"

	_ "time/tzdata"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/factory"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
)

func greenAPISettings() *settings.Settings {
	s := settings.Defaults()
	s.Provider = settings.ProviderGreenAPI
	s.GreenAPI.IDInstance = "7103123456"
	s.GreenAPI.APIToken = "token"
	return s
}

func gowaSettings() *settings.Settings {
	s := settings.Defaults()
	s.Provider = settings.ProviderGOWA
	s.GOWA.BaseURL = "http://gowa:3000"
	return s
}

func newTransport() *provider.Transport {
	return provider.NewTransport(provider.TransportOptions{})
}

func TestBuildGreenAPI(t *testing.T) {
	p, err := factory.Build(greenAPISettings(), newTransport(), nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if p.Name() != "greenapi" {
		t.Errorf("name = %q", p.Name())
	}
}

func TestBuildGOWA(t *testing.T) {
	p, err := factory.Build(gowaSettings(), newTransport(), nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if p.Name() != "gowa" {
		t.Errorf("name = %q", p.Name())
	}
}

func TestBuildUnknownProvider(t *testing.T) {
	s := settings.Defaults()
	s.Provider = "carrier-pigeon"
	if _, err := factory.Build(s, newTransport(), nil); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestBuildMissingCredentialsErrors(t *testing.T) {
	s := settings.Defaults()
	s.Provider = settings.ProviderGreenAPI
	if _, err := factory.Build(s, newTransport(), nil); err == nil {
		t.Fatal("greenapi with no credentials accepted")
	}

	s = settings.Defaults()
	s.Provider = settings.ProviderGOWA
	if _, err := factory.Build(s, newTransport(), nil); err == nil {
		t.Fatal("gowa with no base url accepted")
	}
}

func TestBuildRejectsNilSettings(t *testing.T) {
	if _, err := factory.Build(nil, newTransport(), nil); err == nil {
		t.Fatal("nil settings accepted")
	}
}

func TestRebuildSwapsActiveProviderWithoutRestart(t *testing.T) {
	mgr := provider.NewManager()
	tr := newTransport()

	if err := factory.Rebuild(mgr, greenAPISettings(), tr, nil); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	if mgr.Name() != "greenapi" {
		t.Fatalf("name = %q, want greenapi", mgr.Name())
	}

	// The spec requires switching providers without restarting the process.
	if err := factory.Rebuild(mgr, gowaSettings(), tr, nil); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	if mgr.Name() != "gowa" {
		t.Errorf("name = %q, want gowa after the swap", mgr.Name())
	}
}

func TestRebuildLeavesManagerUntouchedOnError(t *testing.T) {
	mgr := provider.NewManager()
	tr := newTransport()

	if err := factory.Rebuild(mgr, greenAPISettings(), tr, nil); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}

	broken := settings.Defaults()
	broken.Provider = settings.ProviderGOWA // no base url
	if err := factory.Rebuild(mgr, broken, tr, nil); err == nil {
		t.Fatal("rebuild with broken settings succeeded")
	}

	// Losing a working provider because of a bad edit would be worse than
	// rejecting the edit.
	if mgr.Name() != "greenapi" {
		t.Errorf("name = %q; a failed rebuild must not clear the working provider", mgr.Name())
	}
}

func TestBuiltProviderIsRateLimited(t *testing.T) {
	p, err := factory.Build(gowaSettings(), newTransport(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// The rate limiter is a decorator, so the concrete client type must not
	// surface directly — otherwise sends bypass the pacing.
	if _, isRaw := p.(interface{ BaseURL() string }); isRaw {
		t.Error("factory returned the bare client; outbound sends would not be paced")
	}
}
