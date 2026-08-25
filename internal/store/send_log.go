package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	defaultSendLogLimit = 100
	maxSendLogLimit     = 500
)

const sendLogColumns = `id, kind, contact_id, group_id, blessing_id, provider,
	chat_id, status, error, event_year, sent_at`

// AppendSendLog records one send attempt.
//
// A second successful scheduled send for the same (contact, event_year) is
// rejected by the partial unique index in migration 0001 — that index, not
// in-memory state, is what guarantees one blessing per contact per year.
func (s *Store) AppendSendLog(ctx context.Context, e *SendLogEntry) (*SendLogEntry, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO send_log (kind, contact_id, group_id, blessing_id, provider,
			chat_id, status, error, event_year, sent_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Kind, e.ContactID, e.GroupID, e.BlessingID, e.Provider,
		e.ChatID, e.Status, e.Error, e.EventYear, formatTime(time.Now().UTC()),
	)
	if err != nil {
		return nil, fmt.Errorf("appending send log: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading send log id: %w", err)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sendLogColumns+` FROM send_log WHERE id = ?`, id)
	return scanSendLog(row)
}

// HasSuccessfulScheduledSend reports whether this contact already received a
// scheduled blessing for the given event year. Only 'sent' rows count, so a
// failed attempt leaves the scheduler free to retry (spec §6).
func (s *Store) HasSuccessfulScheduledSend(ctx context.Context, contactID int64, eventYear int) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM send_log
		 WHERE kind = ? AND status = ? AND contact_id = ? AND event_year = ?
		 LIMIT 1`,
		KindScheduled, StatusSent, contactID, eventYear,
	).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("checking scheduled send for contact %d year %d: %w",
			contactID, eventYear, err)
	default:
		return true, nil
	}
}

// LastGroupEcho returns the most recent group-echo entry for a group, or
// ErrNotFound. The echo service uses its timestamp for the cooldown check.
func (s *Store) LastGroupEcho(ctx context.Context, groupID int64) (*SendLogEntry, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sendLogColumns+` FROM send_log
		 WHERE kind = ? AND group_id = ?
		 ORDER BY sent_at DESC, id DESC LIMIT 1`,
		KindGroupEcho, groupID)
	return scanSendLog(row)
}

// LastScheduledBlessing returns the blessing used for a contact's most recent
// successful scheduled send, or nil when there is no history. The selector uses
// it to bias away from repeating last year's message (spec §6).
func (s *Store) LastScheduledBlessing(ctx context.Context, contactID int64) (*int64, error) {
	var blessingID sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT blessing_id FROM send_log
		 WHERE kind = ? AND status = ? AND contact_id = ?
		 ORDER BY sent_at DESC, id DESC LIMIT 1`,
		KindScheduled, StatusSent, contactID).Scan(&blessingID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reading last blessing for contact %d: %w", contactID, err)
	}
	if !blessingID.Valid {
		return nil, nil
	}
	v := blessingID.Int64
	return &v, nil
}

// ListSendLog returns the history feed, newest first. An empty kind means all
// kinds; a limit of zero or less uses the default, and anything larger than the
// maximum is clamped.
func (s *Store) ListSendLog(ctx context.Context, kind string, limit int) ([]SendLogEntry, error) {
	if limit <= 0 {
		limit = defaultSendLogLimit
	}
	if limit > maxSendLogLimit {
		limit = maxSendLogLimit
	}

	query := `SELECT ` + sendLogColumns + ` FROM send_log`
	args := make([]any, 0, 2)
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY sent_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing send log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SendLogEntry, 0)
	for rows.Next() {
		e, err := scanSendLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating send log: %w", err)
	}
	return out, nil
}

func scanSendLog(sc scanner) (*SendLogEntry, error) {
	var (
		e          SendLogEntry
		contactID  sql.NullInt64
		groupID    sql.NullInt64
		blessingID sql.NullInt64
		errMsg     sql.NullString
		eventYear  sql.NullInt64
		sentAt     string
	)
	err := sc.Scan(&e.ID, &e.Kind, &contactID, &groupID, &blessingID, &e.Provider,
		&e.ChatID, &e.Status, &errMsg, &eventYear, &sentAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning send log entry: %w", err)
	}
	if contactID.Valid {
		v := contactID.Int64
		e.ContactID = &v
	}
	if groupID.Valid {
		v := groupID.Int64
		e.GroupID = &v
	}
	if blessingID.Valid {
		v := blessingID.Int64
		e.BlessingID = &v
	}
	if errMsg.Valid {
		v := errMsg.String
		e.Error = &v
	}
	if eventYear.Valid {
		v := int(eventYear.Int64)
		e.EventYear = &v
	}
	if e.SentAt, err = parseTime(sentAt); err != nil {
		return nil, err
	}
	return &e, nil
}
