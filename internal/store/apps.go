package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

type App struct {
	ID         int64
	Name       string
	APIKeyHash string
	CreatedAt  time.Time
}

func (s *Store) CreateApp(ctx context.Context, name, apiKeyHash string) (App, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO apps (name, api_key_hash, created_at) VALUES (?, ?, ?)`,
		name, apiKeyHash, fmtTime(now))
	if err != nil {
		return App{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return App{}, err
	}
	return App{ID: id, Name: name, APIKeyHash: apiKeyHash, CreatedAt: now}, nil
}

func (s *Store) ListApps(ctx context.Context) ([]App, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, api_key_hash, created_at FROM apps ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []App
	for rows.Next() {
		var a App
		var created string
		if err := rows.Scan(&a.ID, &a.Name, &a.APIKeyHash, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(created)
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (s *Store) GetAppByID(ctx context.Context, id int64) (App, error) {
	return scanApp(s.db.QueryRowContext(ctx,
		`SELECT id, name, api_key_hash, created_at FROM apps WHERE id = ?`, id))
}

func (s *Store) GetAppByKeyHash(ctx context.Context, keyHash string) (App, error) {
	return scanApp(s.db.QueryRowContext(ctx,
		`SELECT id, name, api_key_hash, created_at FROM apps WHERE api_key_hash = ?`, keyHash))
}

func scanApp(row *sql.Row) (App, error) {
	var a App
	var created string
	err := row.Scan(&a.ID, &a.Name, &a.APIKeyHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return App{}, ErrNotFound
	}
	if err != nil {
		return App{}, err
	}
	a.CreatedAt = parseTime(created)
	return a, nil
}

func (s *Store) RotateAppKey(ctx context.Context, id int64, newKeyHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE apps SET api_key_hash = ? WHERE id = ?`, newKeyHash, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteApp(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
	return err
}
