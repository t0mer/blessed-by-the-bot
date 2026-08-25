package provider_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

type stubProvider struct {
	name       string
	lastChatID string
	lastText   string
	sends      int
	statusCall int
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) SendText(_ context.Context, chatID, text string) (string, error) {
	s.sends++
	s.lastChatID, s.lastText = chatID, text
	return "msg-" + s.name, nil
}

func (s *stubProvider) ListGroups(_ context.Context) ([]provider.Group, error) {
	return []provider.Group{{ChatID: "g@g.us", Name: s.name}}, nil
}

func (s *stubProvider) Status(_ context.Context) (provider.ProviderStatus, error) {
	s.statusCall++
	return provider.ProviderStatus{Provider: s.name, Connected: true, State: "ok"}, nil
}

func TestManagerActiveWithoutProvider(t *testing.T) {
	m := provider.NewManager()
	if _, err := m.Active(); !errors.Is(err, provider.ErrNoProvider) {
		t.Fatalf("err = %v, want ErrNoProvider", err)
	}
	if m.Name() != "" {
		t.Errorf("Name() = %q, want empty", m.Name())
	}
}

func TestManagerSetThenActive(t *testing.T) {
	m := provider.NewManager()
	stub := &stubProvider{name: "greenapi"}
	m.Set(stub)

	got, err := m.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if got != provider.Provider(stub) {
		t.Error("Active() returned a different instance")
	}
	if m.Name() != "greenapi" {
		t.Errorf("Name() = %q, want greenapi", m.Name())
	}
}

func TestManagerSwapsWithoutRestart(t *testing.T) {
	m := provider.NewManager()
	m.Set(&stubProvider{name: "greenapi"})
	m.Set(&stubProvider{name: "gowa"})

	if m.Name() != "gowa" {
		t.Fatalf("Name() = %q, want gowa after the swap", m.Name())
	}
	active, err := m.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.Name() != "gowa" {
		t.Errorf("active provider = %q, want gowa", active.Name())
	}
}

func TestManagerClear(t *testing.T) {
	m := provider.NewManager()
	m.Set(&stubProvider{name: "gowa"})
	m.Clear()
	if _, err := m.Active(); !errors.Is(err, provider.ErrNoProvider) {
		t.Fatalf("err = %v, want ErrNoProvider after Clear", err)
	}
}

func TestManagerIsSafeUnderConcurrency(t *testing.T) {
	m := provider.NewManager()
	m.Set(&stubProvider{name: "greenapi"})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				m.Set(&stubProvider{name: "greenapi"})
			} else {
				m.Set(&stubProvider{name: "gowa"})
			}
		}(i)
		go func() {
			defer wg.Done()
			if p, err := m.Active(); err == nil {
				_ = p.Name()
			}
		}()
	}
	wg.Wait()

	// The manager must still hold a usable provider after the storm.
	active, err := m.Active()
	if err != nil {
		t.Fatalf("active after concurrent swaps: %v", err)
	}
	if name := active.Name(); name != "greenapi" && name != "gowa" {
		t.Errorf("active provider = %q, want one of the two stubs", name)
	}
}
