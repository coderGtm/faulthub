// Package store persists FaultHub data in SQLite.
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS apps (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			api_key_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS issues (
			id INTEGER PRIMARY KEY,
			app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			stack_trace_hash TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','resolved')),
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			UNIQUE(app_id, stack_trace_hash)
		)`,
		`CREATE TABLE IF NOT EXISTS reports (
			id TEXT PRIMARY KEY,
			app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
			installation_id TEXT,
			package_name TEXT,
			app_version_code TEXT,
			app_version_name TEXT,
			android_version TEXT,
			brand TEXT,
			phone_model TEXT,
			product TEXT,
			thread_id INTEGER NOT NULL DEFAULT 0,
			thread_name TEXT,
			thread_priority INTEGER NOT NULL DEFAULT 0,
			thread_group TEXT,
			stack_trace TEXT,
			stack_trace_hash TEXT NOT NULL,
			user_comment TEXT,
			user_email TEXT,
			user_app_start_date TEXT,
			user_crash_date TEXT,
			received_at TEXT NOT NULL,
			raw TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reports_app_time ON reports(app_id, received_at)`,
		`CREATE INDEX IF NOT EXISTS idx_reports_issue ON reports(issue_id)`,
		`CREATE INDEX IF NOT EXISTS idx_reports_install ON reports(installation_id)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			expires_at TEXT NOT NULL
		)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
