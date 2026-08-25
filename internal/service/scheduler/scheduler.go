package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/service/blessing"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// DefaultInterval is how often the loop looks for due contacts. A minute is
// fine-grained enough for an HH:MM send time and cheap enough to run forever.
const DefaultInterval = time.Minute

// Deps are the collaborators the scheduler needs.
type Deps struct {
	Store     *store.Store
	Settings  *settings.Service
	Providers *provider.Manager
	Blessings *blessing.Selector
	Logger    *slog.Logger

	// Now is the clock, injectable so tests can freeze it. Defaults to time.Now.
	Now func() time.Time
	// Interval overrides DefaultInterval.
	Interval time.Duration
}

// Scheduler sends blessings to contacts whose event is today.
type Scheduler struct {
	store     *store.Store
	settings  *settings.Service
	providers *provider.Manager
	blessings *blessing.Selector
	log       *slog.Logger
	now       func() time.Time
	interval  time.Duration
}

// New validates deps and returns a Scheduler.
func New(deps Deps) (*Scheduler, error) {
	switch {
	case deps.Store == nil:
		return nil, errors.New("scheduler: store is required")
	case deps.Settings == nil:
		return nil, errors.New("scheduler: settings service is required")
	case deps.Providers == nil:
		return nil, errors.New("scheduler: provider manager is required")
	case deps.Blessings == nil:
		return nil, errors.New("scheduler: blessing selector is required")
	case deps.Logger == nil:
		return nil, errors.New("scheduler: logger is required")
	}

	now := deps.Now
	if now == nil {
		now = time.Now
	}
	interval := deps.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Scheduler{
		store: deps.Store, settings: deps.Settings, providers: deps.Providers,
		blessings: deps.Blessings, log: deps.Logger, now: now, interval: interval,
	}, nil
}

// Providers exposes the manager so callers and tests can swap the active backend.
func (s *Scheduler) Providers() *provider.Manager { return s.providers }

// Tick runs one pass over the contacts. It returns an error only for faults that
// make the whole pass meaningless — an unreadable settings row or contact list.
// A single contact failing is logged and recorded, never fatal: one missing
// template must not stop everyone else's birthday.
func (s *Scheduler) Tick(ctx context.Context) error {
	current, err := s.settings.Load(ctx)
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}

	loc, err := time.LoadLocation(current.Scheduler.Timezone)
	if err != nil {
		return fmt.Errorf("loading timezone %q: %w", current.Scheduler.Timezone, err)
	}
	now := s.now().In(loc)

	contacts, err := s.store.ListContacts(ctx)
	if err != nil {
		return fmt.Errorf("listing contacts: %w", err)
	}

	for i := range contacts {
		c := &contacts[i]
		due, dueErr := s.isDue(ctx, c, current.Scheduler.SendTime, now)
		if dueErr != nil {
			s.log.Error("evaluating contact", "contact_id", c.ID, "error", dueErr)
			continue
		}
		if !due {
			continue
		}
		if _, sendErr := s.send(ctx, c, now.Year()); sendErr != nil {
			s.log.Error("sending scheduled blessing", "contact_id", c.ID, "error", sendErr)
		}
	}
	return nil
}

// Run ticks until ctx is cancelled.
//
// The first tick fires immediately rather than after one interval, so a restart
// picks up a send whose time has already passed instead of waiting a minute.
// A failing tick is logged, never fatal: taking the process down would take the
// settings UI with it, and the UI is where the operator fixes the cause.
func (s *Scheduler) Run(ctx context.Context) error {
	s.log.Info("scheduler started", "interval", s.interval)
	defer s.log.Info("scheduler stopped")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("scheduler tick failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// isDue applies the three conditions from spec §6: the event falls today, the
// effective send time has passed, and no successful send exists for this year.
//
// The last check reads the send log rather than any in-memory state, which is
// what makes a restart mid-day safe.
func (s *Scheduler) isDue(ctx context.Context, c *store.Contact, generalSendTime string, now time.Time) (bool, error) {
	if !c.Enabled {
		return false, nil
	}

	occurs, err := DueOn(c.EventDate, now)
	if err != nil || !occurs {
		return false, err
	}

	passed, err := ClockPassed(EffectiveSendTime(c, generalSendTime), now)
	if err != nil || !passed {
		return false, err
	}

	already, err := s.store.HasSuccessfulScheduledSend(ctx, c.ID, now.Year())
	if err != nil {
		return false, err
	}
	return !already, nil
}

// send delivers this year's scheduled blessing, recorded against eventYear so
// the unique index enforces one per contact per year.
func (s *Scheduler) send(ctx context.Context, c *store.Contact, eventYear int) (*store.SendLogEntry, error) {
	return s.deliver(ctx, c, &eventYear)
}

// deliver picks a blessing, sends it and records the outcome.
//
// A delivery failure is returned to the caller for logging but is also written
// to the send log with status 'failed'. Only a successful row counts toward the
// yearly dedupe, so the next tick retries — for the rest of the day.
func (s *Scheduler) deliver(ctx context.Context, c *store.Contact, eventYear *int) (*store.SendLogEntry, error) {
	chosen, err := s.blessings.ForContact(ctx, c)
	if err != nil {
		return nil, err
	}

	chatID, err := provider.NormalizeChatID(c.Phone)
	if err != nil {
		return nil, fmt.Errorf("building chat id for contact %d: %w", c.ID, err)
	}

	active, err := s.providers.Active()
	if err != nil {
		return nil, err
	}

	text := blessing.Render(chosen.Text, c)
	entry := &store.SendLogEntry{
		Kind: store.KindScheduled, ContactID: &c.ID, BlessingID: &chosen.ID,
		Provider: active.Name(), ChatID: chatID, EventYear: eventYear,
		SentAt: s.now().UTC(),
	}

	if _, sendErr := active.SendText(ctx, chatID, text); sendErr != nil {
		reason := sendErr.Error()
		entry.Status, entry.Error = store.StatusFailed, &reason
		logged, logErr := s.store.AppendSendLog(ctx, entry)
		if logErr != nil {
			return nil, fmt.Errorf("recording failed send: %w", logErr)
		}
		return logged, sendErr
	}

	entry.Status = store.StatusSent
	logged, err := s.store.AppendSendLog(ctx, entry)
	if err != nil {
		// The message is already out. Failing to record it risks a duplicate on
		// the next tick, so this is loud.
		return nil, fmt.Errorf("recording successful send to contact %d: %w", c.ID, err)
	}
	s.log.Info("blessing sent",
		"contact_id", c.ID, "blessing_id", chosen.ID, "provider", active.Name())
	return logged, nil
}
