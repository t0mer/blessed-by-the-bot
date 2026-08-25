package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func i64Ptr(i int64) *int64 { return &i }

func scheduledSend(contactID int64, year int, status string) *store.SendLogEntry {
	return &store.SendLogEntry{
		Kind:      store.KindScheduled,
		ContactID: i64Ptr(contactID),
		Provider:  "greenapi",
		ChatID:    "972501234567@c.us",
		Status:    status,
		EventYear: &year,
	}
}

func TestAppendSendLogRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	contact, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	blessing, err := s.CreateBlessing(ctx, sampleBlessing())
	if err != nil {
		t.Fatalf("create blessing: %v", err)
	}

	failure := "provider timeout"
	year := 2026
	got, err := s.AppendSendLog(ctx, &store.SendLogEntry{
		Kind:       store.KindScheduled,
		ContactID:  &contact.ID,
		BlessingID: &blessing.ID,
		Provider:   "gowa",
		ChatID:     "972501234567@c.us",
		Status:     store.StatusFailed,
		Error:      &failure,
		EventYear:  &year,
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if got.ID == 0 || got.SentAt.IsZero() {
		t.Errorf("id/sent_at not populated: %+v", got)
	}
	if got.ContactID == nil || *got.ContactID != contact.ID {
		t.Errorf("contact id = %v, want %d", got.ContactID, contact.ID)
	}
	if got.BlessingID == nil || *got.BlessingID != blessing.ID {
		t.Errorf("blessing id = %v, want %d", got.BlessingID, blessing.ID)
	}
	if got.GroupID != nil {
		t.Errorf("group id = %v, want nil", *got.GroupID)
	}
	if got.Error == nil || *got.Error != failure {
		t.Errorf("error = %v, want %q", got.Error, failure)
	}
	if got.EventYear == nil || *got.EventYear != 2026 {
		t.Errorf("event year = %v, want 2026", got.EventYear)
	}
}

func TestScheduledDedupeIndexBlocksSecondSuccess(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	contact, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.AppendSendLog(ctx, scheduledSend(contact.ID, 2026, store.StatusSent)); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if _, err := s.AppendSendLog(ctx, scheduledSend(contact.ID, 2026, store.StatusSent)); err == nil {
		t.Fatal("a second successful scheduled send for the same contact and year was accepted")
	}
}

func TestScheduledDedupeAllowsFailedRetries(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	contact, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatal(err)
	}

	// Failures retry on subsequent ticks the same day (spec §6): only 'sent'
	// rows participate in the dedupe index.
	for i := 0; i < 3; i++ {
		if _, err := s.AppendSendLog(ctx, scheduledSend(contact.ID, 2026, store.StatusFailed)); err != nil {
			t.Fatalf("failed attempt %d rejected: %v", i, err)
		}
	}
	if _, err := s.AppendSendLog(ctx, scheduledSend(contact.ID, 2026, store.StatusSent)); err != nil {
		t.Fatalf("success after failures rejected: %v", err)
	}
}

func TestScheduledDedupeAllowsDifferentYears(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	contact, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatal(err)
	}
	for _, year := range []int{2025, 2026, 2027} {
		if _, err := s.AppendSendLog(ctx, scheduledSend(contact.ID, year, store.StatusSent)); err != nil {
			t.Fatalf("year %d rejected: %v", year, err)
		}
	}
}

func TestHasSuccessfulScheduledSend(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	contact, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatal(err)
	}

	has, err := s.HasSuccessfulScheduledSend(ctx, contact.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("reported a send before anything was logged")
	}

	if _, err := s.AppendSendLog(ctx, scheduledSend(contact.ID, 2026, store.StatusFailed)); err != nil {
		t.Fatal(err)
	}
	has, err = s.HasSuccessfulScheduledSend(ctx, contact.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("a failed attempt counted as sent; the scheduler would never retry")
	}

	if _, err := s.AppendSendLog(ctx, scheduledSend(contact.ID, 2026, store.StatusSent)); err != nil {
		t.Fatal(err)
	}
	has, err = s.HasSuccessfulScheduledSend(ctx, contact.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("successful send not reported; the contact would be messaged twice")
	}
}

func TestLastGroupEchoReturnsMostRecent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	g := newGroup(t, s, "g1@g.us")

	if _, err := s.LastGroupEcho(ctx, g.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("with no history: err = %v, want ErrNotFound", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := s.AppendSendLog(ctx, &store.SendLogEntry{
			Kind:     store.KindGroupEcho,
			GroupID:  &g.ID,
			Provider: "gowa",
			ChatID:   g.ChatID,
			Status:   store.StatusSent,
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	last, err := s.LastGroupEcho(ctx, g.ID)
	if err != nil {
		t.Fatalf("last echo: %v", err)
	}
	var maxID int64
	if err := s.DB().QueryRow(`SELECT max(id) FROM send_log`).Scan(&maxID); err != nil {
		t.Fatal(err)
	}
	if last.ID != maxID {
		t.Errorf("returned id %d, want the most recent %d", last.ID, maxID)
	}
}

func TestLastScheduledBlessing(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	contact, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatal(err)
	}
	blessing, err := s.CreateBlessing(ctx, sampleBlessing())
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.LastScheduledBlessing(ctx, contact.ID)
	if err != nil {
		t.Fatalf("without history: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil without history", *got)
	}

	entry := scheduledSend(contact.ID, 2026, store.StatusSent)
	entry.BlessingID = &blessing.ID
	if _, err := s.AppendSendLog(ctx, entry); err != nil {
		t.Fatal(err)
	}

	got, err = s.LastScheduledBlessing(ctx, contact.ID)
	if err != nil {
		t.Fatalf("with history: %v", err)
	}
	if got == nil || *got != blessing.ID {
		t.Errorf("got %v, want %d", got, blessing.ID)
	}
}

func TestListSendLogFiltersByKindAndAppliesLimit(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	contact, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatal(err)
	}
	g := newGroup(t, s, "g1@g.us")

	for _, year := range []int{2023, 2024, 2025} {
		if _, err := s.AppendSendLog(ctx, scheduledSend(contact.ID, year, store.StatusSent)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := s.AppendSendLog(ctx, &store.SendLogEntry{
		Kind: store.KindGroupEcho, GroupID: &g.ID, Provider: "gowa",
		ChatID: g.ChatID, Status: store.StatusSent,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListSendLog(ctx, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Errorf("got %d entries, want 4", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].SentAt.After(all[i-1].SentAt) {
			t.Errorf("feed is not newest-first at position %d", i)
		}
	}

	scheduled, err := s.ListSendLog(ctx, store.KindScheduled, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduled) != 3 {
		t.Errorf("got %d scheduled entries, want 3", len(scheduled))
	}

	limited, err := s.ListSendLog(ctx, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Errorf("got %d entries with limit 2, want 2", len(limited))
	}

	empty, err := s.ListSendLog(ctx, store.KindGroupEcho, 0)
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil {
		t.Error("empty result is nil; JSON encodes nil as null, want []")
	}
}
