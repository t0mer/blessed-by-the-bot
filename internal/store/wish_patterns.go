package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const wishPatternColumns = `id, language, pattern, enabled`

// CreateWishPattern inserts p and returns it with its assigned ID.
func (s *Store) CreateWishPattern(ctx context.Context, p *WishPattern) (*WishPattern, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO wish_patterns (language, pattern, enabled) VALUES (?, ?, ?)`,
		p.Language, p.Pattern, boolToInt(p.Enabled),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting wish pattern: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading wish pattern id: %w", err)
	}
	return s.GetWishPattern(ctx, id)
}

// GetWishPattern returns the pattern with the given id, or ErrNotFound.
func (s *Store) GetWishPattern(ctx context.Context, id int64) (*WishPattern, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+wishPatternColumns+` FROM wish_patterns WHERE id = ?`, id)
	return scanWishPattern(row)
}

// ListWishPatterns returns every pattern, enabled or not.
func (s *Store) ListWishPatterns(ctx context.Context) ([]WishPattern, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+wishPatternColumns+` FROM wish_patterns ORDER BY language, id`)
	if err != nil {
		return nil, fmt.Errorf("listing wish patterns: %w", err)
	}
	return collectWishPatterns(rows)
}

// ListEnabledWishPatterns returns the patterns the echo service matches against.
// Groups are multilingual, so this deliberately does not filter by language.
func (s *Store) ListEnabledWishPatterns(ctx context.Context) ([]WishPattern, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+wishPatternColumns+` FROM wish_patterns WHERE enabled = 1 ORDER BY language, id`)
	if err != nil {
		return nil, fmt.Errorf("listing enabled wish patterns: %w", err)
	}
	return collectWishPatterns(rows)
}

// UpdateWishPattern writes every mutable field of p.
func (s *Store) UpdateWishPattern(ctx context.Context, p *WishPattern) (*WishPattern, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE wish_patterns SET language = ?, pattern = ?, enabled = ? WHERE id = ?`,
		p.Language, p.Pattern, boolToInt(p.Enabled), p.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating wish pattern %d: %w", p.ID, err)
	}
	if err := requireAffected(res, "wish pattern", p.ID); err != nil {
		return nil, err
	}
	return s.GetWishPattern(ctx, p.ID)
}

// DeleteWishPattern removes the pattern with the given id.
func (s *Store) DeleteWishPattern(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM wish_patterns WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting wish pattern %d: %w", id, err)
	}
	return requireAffected(res, "wish pattern", id)
}

func collectWishPatterns(rows *sql.Rows) ([]WishPattern, error) {
	defer func() { _ = rows.Close() }()

	out := make([]WishPattern, 0)
	for rows.Next() {
		p, err := scanWishPattern(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating wish patterns: %w", err)
	}
	return out, nil
}

func scanWishPattern(sc scanner) (*WishPattern, error) {
	var (
		p       WishPattern
		enabled int
	)
	err := sc.Scan(&p.ID, &p.Language, &p.Pattern, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning wish pattern: %w", err)
	}
	p.Enabled = enabled != 0
	return &p, nil
}
