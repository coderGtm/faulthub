package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) CreateSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, expires_at) VALUES (?, ?)`,
		tokenHash, fmtTime(expiresAt))
	return err
}

func (s *Store) ValidSession(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var expires string
	err := s.db.QueryRowContext(ctx,
		`SELECT expires_at FROM sessions WHERE token_hash = ?`, tokenHash).Scan(&expires)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return parseTime(expires).After(now), nil
}

func (s *Store) TouchSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`,
		fmtTime(expiresAt), tokenHash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) PruneSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, fmtTime(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
