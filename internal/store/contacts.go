package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const contactColumns = `id, name, phone, event_date, event_type, language,
	relation, importance, gender, send_time, enabled, created_at, updated_at`

// CreateContact inserts c and returns it with ID and timestamps populated.
func (s *Store) CreateContact(ctx context.Context, c *Contact) (*Contact, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO contacts (name, phone, event_date, event_type, language,
			relation, importance, gender, send_time, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Phone, c.EventDate, c.EventType, c.Language, c.Relation,
		c.Importance, c.Gender, c.SendTime, boolToInt(c.Enabled),
		formatTime(now), formatTime(now),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting contact: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading contact id: %w", err)
	}
	return s.GetContact(ctx, id)
}

// GetContact returns the contact with the given id, or ErrNotFound.
func (s *Store) GetContact(ctx context.Context, id int64) (*Contact, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+contactColumns+` FROM contacts WHERE id = ?`, id)
	return scanContact(row)
}

// ListContacts returns every contact ordered by name.
func (s *Store) ListContacts(ctx context.Context) ([]Contact, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+contactColumns+` FROM contacts ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("listing contacts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Contact, 0)
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating contacts: %w", err)
	}
	return out, nil
}

// UpdateContact writes every mutable field of c and refreshes updated_at.
func (s *Store) UpdateContact(ctx context.Context, c *Contact) (*Contact, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE contacts SET name = ?, phone = ?, event_date = ?, event_type = ?,
			language = ?, relation = ?, importance = ?, gender = ?, send_time = ?,
			enabled = ?, updated_at = ?
		 WHERE id = ?`,
		c.Name, c.Phone, c.EventDate, c.EventType, c.Language, c.Relation,
		c.Importance, c.Gender, c.SendTime, boolToInt(c.Enabled),
		formatTime(time.Now().UTC()), c.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating contact %d: %w", c.ID, err)
	}
	if err := requireAffected(res, "contact", c.ID); err != nil {
		return nil, err
	}
	return s.GetContact(ctx, c.ID)
}

// DeleteContact removes the contact with the given id.
func (s *Store) DeleteContact(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM contacts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting contact %d: %w", id, err)
	}
	return requireAffected(res, "contact", id)
}

func scanContact(sc scanner) (*Contact, error) {
	var (
		c         Contact
		sendTime  sql.NullString
		enabled   int
		createdAt string
		updatedAt string
	)
	err := sc.Scan(&c.ID, &c.Name, &c.Phone, &c.EventDate, &c.EventType, &c.Language,
		&c.Relation, &c.Importance, &c.Gender, &sendTime, &enabled, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning contact: %w", err)
	}
	if sendTime.Valid {
		v := sendTime.String
		c.SendTime = &v
	}
	c.Enabled = enabled != 0
	if c.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}
