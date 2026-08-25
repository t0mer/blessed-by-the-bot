package store

import (
	"context"
	"fmt"
	"time"
)

// RecordWishEvent stores one wish sighting and reports whether it was new.
//
// The UNIQUE(group_id, message_id) constraint makes redelivery of the same
// provider message a no-op, so a webhook retry cannot inflate the trigger count.
func (s *Store) RecordWishEvent(ctx context.Context, e *WishEvent) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO wish_events (group_id, sender_id, message_id, matched, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (group_id, message_id) DO NOTHING`,
		e.GroupID, e.SenderID, e.MessageID, e.Matched, formatTime(time.Now().UTC()),
	)
	if err != nil {
		return false, fmt.Errorf("recording wish event: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("reading rows affected: %w", err)
	}
	return n > 0, nil
}

// CountDistinctWishSenders counts how many different people wished in a group
// since the given time. Distinct senders, not messages: one excited friend
// sending five messages is one vote (spec §2.2).
func (s *Store) CountDistinctWishSenders(ctx context.Context, groupID int64, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(DISTINCT sender_id) FROM wish_events
		 WHERE group_id = ? AND created_at >= ?`,
		groupID, formatTime(since.UTC()),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting wish senders for group %d: %w", groupID, err)
	}
	return n, nil
}

// DeleteWishEventsBefore drops stale evidence and returns how many rows went.
// Called by the nightly janitor (spec §7).
func (s *Store) DeleteWishEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM wish_events WHERE created_at < ?`, formatTime(cutoff.UTC()))
	if err != nil {
		return 0, fmt.Errorf("deleting wish events before %s: %w", cutoff, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reading rows affected: %w", err)
	}
	return n, nil
}
