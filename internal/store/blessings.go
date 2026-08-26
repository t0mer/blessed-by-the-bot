package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const blessingColumns = `id, event_type, language, gender, relation, text,
	enabled, created_at, updated_at`

// CreateBlessing inserts b and returns it with ID and timestamps populated.
func (s *Store) CreateBlessing(ctx context.Context, b *Blessing) (*Blessing, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO blessings (event_type, language, gender, relation, text,
			enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.EventType, b.Language, b.Gender, b.Relation, b.Text,
		boolToInt(b.Enabled), formatTime(now), formatTime(now),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting blessing: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading blessing id: %w", err)
	}
	return s.GetBlessing(ctx, id)
}

// GetBlessing returns the blessing with the given id, or ErrNotFound.
func (s *Store) GetBlessing(ctx context.Context, id int64) (*Blessing, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+blessingColumns+` FROM blessings WHERE id = ?`, id)
	return scanBlessing(row)
}

// ListBlessings returns every blessing grouped by event type and language.
func (s *Store) ListBlessings(ctx context.Context) ([]Blessing, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+blessingColumns+` FROM blessings ORDER BY event_type, language, id`)
	if err != nil {
		return nil, fmt.Errorf("listing blessings: %w", err)
	}
	return collectBlessings(rows)
}

// FindBlessings returns enabled blessings for an event type and language.
// Tier selection (gender and relation fit) is the blessing service's job,
// per spec §6 — this returns the whole candidate set.
func (s *Store) FindBlessings(ctx context.Context, eventType, language string) ([]Blessing, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+blessingColumns+` FROM blessings
		 WHERE enabled = 1 AND event_type = ? AND language = ?
		 ORDER BY id`, eventType, language)
	if err != nil {
		return nil, fmt.Errorf("finding blessings for %s/%s: %w", eventType, language, err)
	}
	return collectBlessings(rows)
}

// UpdateBlessing writes every mutable field of b and refreshes updated_at.
func (s *Store) UpdateBlessing(ctx context.Context, b *Blessing) (*Blessing, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE blessings SET event_type = ?, language = ?, gender = ?, relation = ?,
			text = ?, enabled = ?, updated_at = ?
		 WHERE id = ?`,
		b.EventType, b.Language, b.Gender, b.Relation, b.Text,
		boolToInt(b.Enabled), formatTime(time.Now().UTC()), b.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating blessing %d: %w", b.ID, err)
	}
	if err := requireAffected(res, "blessing", b.ID); err != nil {
		return nil, err
	}
	return s.GetBlessing(ctx, b.ID)
}

// DeleteBlessing removes the blessing with the given id.
func (s *Store) DeleteBlessing(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM blessings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting blessing %d: %w", id, err)
	}
	return requireAffected(res, "blessing", id)
}

func collectBlessings(rows *sql.Rows) ([]Blessing, error) {
	defer func() { _ = rows.Close() }()

	out := make([]Blessing, 0)
	for rows.Next() {
		b, err := scanBlessing(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating blessings: %w", err)
	}
	return out, nil
}

func scanBlessing(sc scanner) (*Blessing, error) {
	var (
		b         Blessing
		gender    sql.NullString
		relation  sql.NullString
		enabled   int
		createdAt string
		updatedAt string
	)
	err := sc.Scan(&b.ID, &b.EventType, &b.Language, &gender, &relation, &b.Text,
		&enabled, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning blessing: %w", err)
	}
	if gender.Valid {
		v := gender.String
		b.Gender = &v
	}
	if relation.Valid {
		v := relation.String
		b.Relation = &v
	}
	b.Enabled = enabled != 0
	if b.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if b.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

// seedBlessingMigrations lists every migration that inserts starter templates,
// in apply order. A new seed migration has to be added here too, or the helper
// below quietly stops restoring the full set a fresh install would have.
var seedBlessingMigrations = []string{
	"migrations/0003_seed_blessings.sql",
	"migrations/0005_seed_custom_name_free_blessings.sql",
}

// ApplySeedBlessings re-runs the starter-template seed migrations.
//
// It exists for tests that clear the table to get a controlled set and then
// need the real seed back to exercise fresh-install behaviour. Running it twice
// duplicates the rows, so it is not something the application calls.
func (s *Store) ApplySeedBlessings(ctx context.Context) error {
	for _, name := range seedBlessingMigrations {
		body, err := migrationsFS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("reading the blessing seed %s: %w", name, err)
		}
		if _, err := s.db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("applying the blessing seed %s: %w", name, err)
		}
	}
	return nil
}
