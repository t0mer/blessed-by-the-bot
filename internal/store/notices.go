package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Notice levels.
const (
	NoticeInfo    = "info"
	NoticeWarning = "warning"
	NoticeError   = "error"
)

// Notice codes. These are part of the API contract: the SPA switches on them to
// choose wording and a call to action.
const (
	// NoticeLanguageFallback means a contact's language had no template and the
	// English one was used instead (spec §6).
	NoticeLanguageFallback = "language_fallback"
)

const noticeColumns = `id, key, level, code, message, detail, occurrences,
	first_seen_at, last_seen_at, dismissed_at`

// RaiseNotice records a condition worth showing in the UI.
//
// Key is the notice's identity: raising the same key again bumps the counter and
// the timestamp instead of adding a row, and clears any previous dismissal —
// the operator dismissed the last occurrence, not the underlying problem, so a
// recurrence should surface again.
func (s *Store) RaiseNotice(ctx context.Context, n Notice) error {
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notices (key, level, code, message, detail, occurrences,
			first_seen_at, last_seen_at, dismissed_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?, NULL)
		 ON CONFLICT (key) DO UPDATE SET
			level        = excluded.level,
			message      = excluded.message,
			detail       = excluded.detail,
			occurrences  = notices.occurrences + 1,
			last_seen_at = excluded.last_seen_at,
			dismissed_at = NULL`,
		n.Key, n.Level, n.Code, n.Message, n.Detail, now, now,
	)
	if err != nil {
		return fmt.Errorf("raising notice %q: %w", n.Key, err)
	}
	return nil
}

// ListNotices returns notices, newest first. activeOnly excludes dismissed ones.
func (s *Store) ListNotices(ctx context.Context, activeOnly bool) ([]Notice, error) {
	query := `SELECT ` + noticeColumns + ` FROM notices`
	if activeOnly {
		query += ` WHERE dismissed_at IS NULL`
	}
	query += ` ORDER BY last_seen_at DESC, id DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing notices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Notice, 0)
	for rows.Next() {
		n, scanErr := scanNotice(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notices: %w", err)
	}
	return out, nil
}

// DismissNotice marks one notice as read. It stays in the table so a recurrence
// can un-dismiss it rather than starting from scratch.
func (s *Store) DismissNotice(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notices SET dismissed_at = ? WHERE id = ? AND dismissed_at IS NULL`,
		formatTime(time.Now().UTC()), id)
	if err != nil {
		return fmt.Errorf("dismissing notice %d: %w", id, err)
	}
	return requireAffected(res, "notice", id)
}

func scanNotice(sc scanner) (*Notice, error) {
	var (
		n           Notice
		detail      sql.NullString
		firstSeen   string
		lastSeen    string
		dismissedAt sql.NullString
	)
	if err := sc.Scan(&n.ID, &n.Key, &n.Level, &n.Code, &n.Message, &detail,
		&n.Occurrences, &firstSeen, &lastSeen, &dismissedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning notice: %w", err)
	}
	if detail.Valid {
		n.Detail = &detail.String
	}

	var err error
	if n.FirstSeenAt, err = parseTime(firstSeen); err != nil {
		return nil, err
	}
	if n.LastSeenAt, err = parseTime(lastSeen); err != nil {
		return nil, err
	}
	if dismissedAt.Valid {
		t, parseErr := parseTime(dismissedAt.String)
		if parseErr != nil {
			return nil, parseErr
		}
		n.DismissedAt = &t
	}
	return &n, nil
}
