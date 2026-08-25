package scheduler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/service/scheduler"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// Send-now ignores the clock: that is the whole point of the button.
func TestSendNowIgnoresTheSendTime(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 3, 0, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	c := seedContact(t, h.store)

	entry, err := h.sched.SendNow(context.Background(), c.ID, false)
	if err != nil {
		t.Fatalf("SendNow: %v", err)
	}
	if entry.Status != store.StatusSent {
		t.Fatalf("status = %q, want sent", entry.Status)
	}
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want 1", got)
	}
}

// It also ignores the date — "send now" means now, not "if it is their birthday".
func TestSendNowIgnoresTheEventDate(t *testing.T) {
	h := at(t, time.Date(2026, 11, 3, 12, 0, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	c := seedContact(t, h.store)

	if _, err := h.sched.SendNow(context.Background(), c.ID, false); err != nil {
		t.Fatalf("SendNow: %v", err)
	}
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want 1", got)
	}
}

// The yearly dedupe still applies, so the button cannot spam someone.
func TestSendNowRespectsTheYearlyDedupe(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 12, 0, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	c := seedContact(t, h.store)

	if _, err := h.sched.SendNow(context.Background(), c.ID, false); err != nil {
		t.Fatalf("first SendNow: %v", err)
	}
	_, err := h.sched.SendNow(context.Background(), c.ID, false)
	if !errors.Is(err, scheduler.ErrAlreadySent) {
		t.Fatalf("err = %v, want ErrAlreadySent", err)
	}
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want 1", got)
	}
}

func TestSendNowForceBypassesTheDedupe(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 12, 0, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	c := seedContact(t, h.store)

	if _, err := h.sched.SendNow(context.Background(), c.ID, false); err != nil {
		t.Fatalf("first SendNow: %v", err)
	}
	if _, err := h.sched.SendNow(context.Background(), c.ID, true); err != nil {
		t.Fatalf("forced SendNow: %v", err)
	}
	if got := len(h.provider.messages()); got != 2 {
		t.Fatalf("sent %d messages, want 2", got)
	}
}

// A forced resend must not violate the unique index that protects the schedule.
// It is recorded without an event year, so it is history, not a dedupe key.
func TestForcedSendIsLoggedWithoutAnEventYear(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 12, 0, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	c := seedContact(t, h.store)

	if _, err := h.sched.SendNow(context.Background(), c.ID, false); err != nil {
		t.Fatalf("first SendNow: %v", err)
	}
	forced, err := h.sched.SendNow(context.Background(), c.ID, true)
	if err != nil {
		t.Fatalf("forced SendNow: %v", err)
	}
	if forced.EventYear != nil {
		t.Fatalf("event_year = %v, want nil for a forced resend", *forced.EventYear)
	}

	// The scheduled send for this year still counts, so the tick stays quiet.
	already, err := h.store.HasSuccessfulScheduledSend(context.Background(), c.ID, 2026)
	if err != nil {
		t.Fatalf("checking dedupe: %v", err)
	}
	if !already {
		t.Fatal("want the original send still recognised for the yearly dedupe")
	}
}

func TestSendNowReportsAMissingContact(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 12, 0, 0, 0, jerusalem(t)))
	if _, err := h.sched.SendNow(context.Background(), 9999, false); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want store.ErrNotFound", err)
	}
}

// A disabled contact is muted deliberately; the button must respect that.
func TestSendNowRefusesADisabledContact(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 12, 0, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	c := seedContact(t, h.store, func(c *store.Contact) { c.Enabled = false })

	if _, err := h.sched.SendNow(context.Background(), c.ID, false); !errors.Is(err, scheduler.ErrContactDisabled) {
		t.Fatalf("err = %v, want ErrContactDisabled", err)
	}
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages for a disabled contact, want 0", got)
	}
}

// After a forced resend the scheduler must still be quiet for the year.
func TestTickStaysQuietAfterAForcedResend(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 12, 0, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	c := seedContact(t, h.store)

	if _, err := h.sched.SendNow(context.Background(), c.ID, false); err != nil {
		t.Fatalf("SendNow: %v", err)
	}
	if _, err := h.sched.SendNow(context.Background(), c.ID, true); err != nil {
		t.Fatalf("forced SendNow: %v", err)
	}
	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := len(h.provider.messages()); got != 2 {
		t.Fatalf("sent %d messages, want 2 — the tick must add none", got)
	}
}
