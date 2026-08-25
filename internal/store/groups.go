package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// "groups" is quoted throughout: GROUPS is a SQLite keyword (window-function
// frames) and quoting costs nothing.
const groupColumns = `id, name, chat_id, language, threshold, enabled,
	created_at, updated_at`

// CreateGroup inserts g and returns it with ID and timestamps populated.
func (s *Store) CreateGroup(ctx context.Context, g *Group) (*Group, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO "groups" (name, chat_id, language, threshold, enabled,
			created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		g.Name, g.ChatID, g.Language, g.Threshold, boolToInt(g.Enabled),
		formatTime(now), formatTime(now),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting group: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading group id: %w", err)
	}
	return s.GetGroup(ctx, id)
}

// GetGroup returns the group with the given id, or ErrNotFound.
func (s *Store) GetGroup(ctx context.Context, id int64) (*Group, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+groupColumns+` FROM "groups" WHERE id = ?`, id)
	return scanGroup(row)
}

// GetGroupByChatID looks a group up by its WhatsApp chat id, which is how
// incoming messages identify it.
func (s *Store) GetGroupByChatID(ctx context.Context, chatID string) (*Group, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+groupColumns+` FROM "groups" WHERE chat_id = ?`, chatID)
	return scanGroup(row)
}

// ListGroups returns every configured group ordered by name.
func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+groupColumns+` FROM "groups" ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Group, 0)
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating groups: %w", err)
	}
	return out, nil
}

// UpdateGroup writes every mutable field of g and refreshes updated_at.
func (s *Store) UpdateGroup(ctx context.Context, g *Group) (*Group, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE "groups" SET name = ?, chat_id = ?, language = ?, threshold = ?,
			enabled = ?, updated_at = ?
		 WHERE id = ?`,
		g.Name, g.ChatID, g.Language, g.Threshold, boolToInt(g.Enabled),
		formatTime(time.Now().UTC()), g.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating group %d: %w", g.ID, err)
	}
	if err := requireAffected(res, "group", g.ID); err != nil {
		return nil, err
	}
	return s.GetGroup(ctx, g.ID)
}

// DeleteGroup removes the group; its wish events cascade away with it.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM "groups" WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting group %d: %w", id, err)
	}
	return requireAffected(res, "group", id)
}

func scanGroup(sc scanner) (*Group, error) {
	var (
		g         Group
		threshold sql.NullInt64
		enabled   int
		createdAt string
		updatedAt string
	)
	err := sc.Scan(&g.ID, &g.Name, &g.ChatID, &g.Language, &threshold, &enabled,
		&createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning group: %w", err)
	}
	if threshold.Valid {
		v := int(threshold.Int64)
		g.Threshold = &v
	}
	g.Enabled = enabled != 0
	if g.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if g.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &g, nil
}
