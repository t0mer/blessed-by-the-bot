// Package listener owns the lifecycle of the GreenAPI polling loop.
//
// Polling is GreenAPI's default incoming mode because it needs no public URL —
// a home-lab deployment behind NAT can still receive messages (spec §4.1). The
// loop itself lives in provider/greenapi; what this package adds is deciding
// when it should be running, and reacting to a settings change without a
// restart (spec §4).
//
// GOWA needs nothing here: it pushes webhooks to /webhooks/gowa instead.
package listener

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/greenapi"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
)

// DefaultInterval is how often the supervisor re-checks settings against what
// is running. A settings change signals Notify for an immediate reconcile, so
// this is only the safety net — it also catches a database edited by hand.
const DefaultInterval = 30 * time.Second

// MessageHandler consumes a polled message. The group-echo engine implements it,
// and it is the same handler the webhook endpoints feed, so a message means the
// same thing however it arrived.
type MessageHandler interface {
	HandleIncoming(ctx context.Context, msg provider.IncomingMessage) error
}

// Deps are the collaborators the supervisor needs.
type Deps struct {
	Settings  *settings.Service
	Transport *provider.Transport
	Incoming  MessageHandler
	Logger    *slog.Logger

	// Interval overrides DefaultInterval.
	Interval time.Duration
	// PollDelay overrides the poller's idle and error pacing. Tests set it small.
	PollDelay time.Duration
}

// Supervisor starts and stops the GreenAPI poller to match current settings.
type Supervisor struct {
	settings  *settings.Service
	transport *provider.Transport
	incoming  MessageHandler
	log       *slog.Logger
	interval  time.Duration
	pollDelay time.Duration

	wake chan struct{}

	mu          sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	fingerprint string
}

// New validates deps and returns a Supervisor.
func New(deps Deps) (*Supervisor, error) {
	switch {
	case deps.Settings == nil:
		return nil, errors.New("listener: settings service is required")
	case deps.Transport == nil:
		return nil, errors.New("listener: transport is required")
	case deps.Logger == nil:
		return nil, errors.New("listener: logger is required")
	}

	interval := deps.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Supervisor{
		settings:  deps.Settings,
		transport: deps.Transport,
		incoming:  deps.Incoming,
		log:       deps.Logger,
		interval:  interval,
		pollDelay: deps.PollDelay,
		// Buffered so Notify never blocks the HTTP handler that calls it.
		wake: make(chan struct{}, 1),
	}, nil
}

// Notify asks for an immediate reconcile. It never blocks: a pending wake-up
// already covers a second request, because reconcile reads settings fresh.
func (s *Supervisor) Notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Polling reports whether a poller is currently running. Exported for tests and
// for a future status endpoint.
func (s *Supervisor) Polling() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel != nil
}

// Run reconciles until ctx is cancelled, then stops any running poller and
// waits for it to finish.
func (s *Supervisor) Run(ctx context.Context) error {
	s.log.Info("listener supervisor started", "interval", s.interval)
	defer s.log.Info("listener supervisor stopped")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		if err := s.reconcile(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("reconciling the incoming-message listener", "error", err)
		}

		select {
		case <-ctx.Done():
			s.stop()
			return nil
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

// reconcile brings the running poller in line with the stored settings.
func (s *Supervisor) reconcile(ctx context.Context) error {
	current, err := s.settings.Load(ctx)
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}

	want, fingerprint := desiredPoller(current)
	if !want {
		s.stop()
		return nil
	}

	s.mu.Lock()
	unchanged := s.cancel != nil && s.fingerprint == fingerprint
	s.mu.Unlock()
	if unchanged {
		return nil
	}

	// Credentials or the endpoint changed: the running poller holds the old ones
	// in its client and would keep failing, so replace it rather than leave it.
	s.stop()
	return s.start(ctx, current, fingerprint)
}

// desiredPoller reports whether a poller should run, and a fingerprint of the
// configuration it would use.
//
// Incomplete credentials deliberately mean "no poller": starting one that can
// only ever fail would bury a real outage in a stream of auth errors.
func desiredPoller(s *settings.Settings) (bool, string) {
	if s.Provider != settings.ProviderGreenAPI {
		return false, ""
	}
	// An empty mode is the documented default, which is polling.
	if s.GreenAPI.Mode != settings.ModePolling && s.GreenAPI.Mode != "" {
		return false, ""
	}
	if strings.TrimSpace(s.GreenAPI.IDInstance) == "" || strings.TrimSpace(s.GreenAPI.APIToken) == "" {
		return false, ""
	}
	return true, strings.Join([]string{s.GreenAPI.APIURL, s.GreenAPI.IDInstance, s.GreenAPI.APIToken}, "|")
}

// start launches a poller under a context derived from ctx.
func (s *Supervisor) start(ctx context.Context, current *settings.Settings, fingerprint string) error {
	client, err := greenapi.New(greenapi.Config{
		APIURL:            current.GreenAPI.APIURL,
		IDInstance:        current.GreenAPI.IDInstance,
		APIToken:          current.GreenAPI.APIToken,
		Mode:              current.GreenAPI.Mode,
		WebhookAuthHeader: current.GreenAPI.WebhookAuthHeader,
	}, s.transport, s.log)
	if err != nil {
		return fmt.Errorf("building the greenapi client: %w", err)
	}

	poller := greenapi.NewPoller(client, s.log, s.dispatch)
	if s.pollDelay > 0 {
		poller.SetIdleDelay(s.pollDelay)
		poller.SetErrorDelay(s.pollDelay)
	}

	pollCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	s.mu.Lock()
	s.cancel = cancel
	s.done = done
	s.fingerprint = fingerprint
	s.mu.Unlock()

	go func() {
		defer close(done)
		if runErr := poller.Run(pollCtx); runErr != nil {
			s.log.Error("greenapi poller exited", "error", runErr)
		}
	}()

	s.log.Info("greenapi polling started", "instance", current.GreenAPI.IDInstance)
	return nil
}

// stop cancels a running poller and waits for it to return, so a replacement
// never overlaps with its predecessor.
func (s *Supervisor) stop() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done, s.fingerprint = nil, nil, ""
	s.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	<-done
	s.log.Info("greenapi polling stopped")
}

// dispatch hands a polled message to the shared incoming handler.
//
// Errors are logged rather than returned: the poller has already consumed and
// acknowledged the receipt, so there is nothing to retry, and a failure here is
// ours to fix rather than the provider's to resend.
func (s *Supervisor) dispatch(ctx context.Context, msg *provider.IncomingMessage) {
	if msg == nil || s.incoming == nil {
		return
	}
	if err := s.incoming.HandleIncoming(ctx, *msg); err != nil {
		s.log.Error("handling a polled message failed",
			"chat_id", msg.ChatID, "error", err)
	}
}
