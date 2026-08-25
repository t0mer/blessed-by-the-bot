package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func newGroup(t *testing.T, s *store.Store, chatID string) *store.Group {
	t.Helper()
	g := sampleGroup()
	g.ChatID = chatID
	created, err := s.CreateGroup(context.Background(), g)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	return created
}

func TestRecordWishEventInserts(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	g := newGroup(t, s, "g1@g.us")

	inserted, err := s.RecordWishEvent(ctx, &store.WishEvent{
		GroupID: g.ID, SenderID: "s1", MessageID: "m1", Matched: "מזל טוב",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if !inserted {
		t.Error("inserted = false on a fresh message")
	}
}

func TestRecordWishEventIsIdempotentPerMessageID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	g := newGroup(t, s, "g1@g.us")

	event := &store.WishEvent{GroupID: g.ID, SenderID: "s1", MessageID: "m1", Matched: "x"}
	if _, err := s.RecordWishEvent(ctx, event); err != nil {
		t.Fatalf("first record: %v", err)
	}
	inserted, err := s.RecordWishEvent(ctx, event)
	if err != nil {
		t.Fatalf("second record: %v", err)
	}
	if inserted {
		t.Error("inserted = true on a redelivered message; a webhook retry would inflate the count")
	}

	var n int
	if err := s.DB().QueryRow(`SELECT count(*) FROM wish_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d rows stored, want 1", n)
	}
}

func TestRecordWishEventSameMessageIDDifferentGroup(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	g1 := newGroup(t, s, "g1@g.us")
	g2 := newGroup(t, s, "g2@g.us")

	for _, id := range []int64{g1.ID, g2.ID} {
		inserted, err := s.RecordWishEvent(ctx, &store.WishEvent{
			GroupID: id, SenderID: "s1", MessageID: "shared-id", Matched: "x",
		})
		if err != nil {
			t.Fatalf("record for group %d: %v", id, err)
		}
		if !inserted {
			t.Errorf("group %d: inserted = false; uniqueness must be per (group, message)", id)
		}
	}
}

func TestCountDistinctWishSendersCountsSendersNotMessages(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	g := newGroup(t, s, "g1@g.us")
	since := time.Now().Add(-time.Hour)

	// One excited friend sending five messages is one vote (spec §2.2).
	for i, msg := range []string{"m1", "m2", "m3", "m4", "m5"} {
		if _, err := s.RecordWishEvent(ctx, &store.WishEvent{
			GroupID: g.ID, SenderID: "loud-friend", MessageID: msg, Matched: "x",
		}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	n, err := s.CountDistinctWishSenders(ctx, g.ID, since)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1 (distinct senders, not messages)", n)
	}

	for i, sender := range []string{"a", "b"} {
		if _, err := s.RecordWishEvent(ctx, &store.WishEvent{
			GroupID: g.ID, SenderID: sender, MessageID: "extra" + sender, Matched: "x",
		}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	n, err = s.CountDistinctWishSenders(ctx, g.ID, since)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Errorf("count = %d, want 3", n)
	}
}

func TestCountDistinctWishSendersRespectsWindow(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	g := newGroup(t, s, "g1@g.us")

	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO wish_events (group_id, sender_id, message_id, matched, created_at)
		 VALUES (?, 'ancient', 'old-msg', 'x', ?)`, g.ID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordWishEvent(ctx, &store.WishEvent{
		GroupID: g.ID, SenderID: "recent", MessageID: "new-msg", Matched: "x",
	}); err != nil {
		t.Fatal(err)
	}

	n, err := s.CountDistinctWishSenders(ctx, g.ID, time.Now().Add(-6*time.Hour))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1; the 48h-old event should fall outside a 6h window", n)
	}
}

func TestDeleteWishEventsBefore(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	g := newGroup(t, s, "g1@g.us")

	old := time.Now().UTC().Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO wish_events (group_id, sender_id, message_id, matched, created_at)
		 VALUES (?, 'ancient', 'old-msg', 'x', ?)`, g.ID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordWishEvent(ctx, &store.WishEvent{
		GroupID: g.ID, SenderID: "recent", MessageID: "new-msg", Matched: "x",
	}); err != nil {
		t.Fatal(err)
	}

	deleted, err := s.DeleteWishEventsBefore(ctx, time.Now().Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("janitor: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	var remaining int
	if err := s.DB().QueryRow(`SELECT count(*) FROM wish_events`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Errorf("%d rows remain, want 1", remaining)
	}
}
