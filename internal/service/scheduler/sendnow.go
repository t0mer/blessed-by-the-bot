package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// ErrAlreadySent means this contact already received their blessing for the
// current event year. force=true overrides it.
var ErrAlreadySent = errors.New("this contact already received a blessing this year")

// ErrContactDisabled means the contact is muted.
var ErrContactDisabled = errors.New("this contact is disabled")

// SendNow delivers a blessing immediately, ignoring both the event date and the
// send time — the operator asked for it, so neither is relevant.
//
// The yearly dedupe still applies unless force is set. A forced send is recorded
// with a nil event year: it is history, but it must not become a second dedupe
// key for the year, and the unique index would reject it anyway.
func (s *Scheduler) SendNow(ctx context.Context, contactID int64, force bool) (*store.SendLogEntry, error) {
	contact, err := s.store.GetContact(ctx, contactID)
	if err != nil {
		return nil, err
	}
	if !contact.Enabled {
		return nil, fmt.Errorf("%w: contact %d", ErrContactDisabled, contactID)
	}

	year, err := s.currentYear(ctx)
	if err != nil {
		return nil, err
	}

	if force {
		return s.sendForced(ctx, contact)
	}

	already, err := s.store.HasSuccessfulScheduledSend(ctx, contactID, year)
	if err != nil {
		return nil, err
	}
	if already {
		return nil, fmt.Errorf("%w: contact %d, year %d", ErrAlreadySent, contactID, year)
	}
	return s.send(ctx, contact, year)
}

// sendForced delivers a deliberate repeat. It carries no event year: the unique
// index would reject a second row for the year, and a forced send is history
// rather than a dedupe key.
func (s *Scheduler) sendForced(ctx context.Context, c *store.Contact) (*store.SendLogEntry, error) {
	return s.deliver(ctx, c, nil)
}

// currentYear resolves "this year" in the configured timezone, which is the same
// year the tick loop uses as the dedupe key.
func (s *Scheduler) currentYear(ctx context.Context) (int, error) {
	current, err := s.settings.Load(ctx)
	if err != nil {
		return 0, fmt.Errorf("loading settings: %w", err)
	}
	loc, err := time.LoadLocation(current.Scheduler.Timezone)
	if err != nil {
		return 0, fmt.Errorf("loading timezone %q: %w", current.Scheduler.Timezone, err)
	}
	return s.now().In(loc).Year(), nil
}
