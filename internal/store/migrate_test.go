package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestOpenCreatesSchema(t *testing.T) {
	st := testStore(t)
	for _, table := range []string{"apps", "issues", "reports", "sessions"} {
		var n int
		err := st.db.QueryRow("SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("table %s missing (n=%d, err=%v)", table, n, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer st.Close()
}

// TestMigrateOldSchema upgrades a database created before the thread
// columns existed: old rows stay readable and new reports with thread
// fields can be ingested.
func TestMigrateOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE apps (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			api_key_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE issues (
			id INTEGER PRIMARY KEY,
			app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			stack_trace_hash TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			UNIQUE(app_id, stack_trace_hash)
		)`,
		`CREATE TABLE reports (
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
			build_fingerprint TEXT,
			thread_details TEXT,
			stack_trace TEXT,
			stack_trace_hash TEXT NOT NULL,
			user_comment TEXT,
			user_email TEXT,
			user_app_start_date TEXT,
			user_crash_date TEXT,
			received_at TEXT NOT NULL,
			raw TEXT NOT NULL
		)`,
		`CREATE TABLE sessions (
			token_hash TEXT PRIMARY KEY,
			expires_at TEXT NOT NULL
		)`,
		`INSERT INTO apps (id, name, api_key_hash, created_at) VALUES (1, 'Old', 'h', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	db.Close()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open old schema: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	r := testReport("old-schema-r1", "hashOld")
	r.AppID = 1
	r.ThreadID, r.ThreadName, r.ThreadPriority, r.ThreadGroup = 2, "main", 5, "g"
	if _, err := st.IngestReport(ctx, r); err != nil {
		t.Fatalf("ingest after migrate: %v", err)
	}
	got, err := st.GetReport(ctx, 1, "old-schema-r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ThreadName != "main" || got.ThreadID != 2 || got.ThreadPriority != 5 || got.ThreadGroup != "g" {
		t.Fatalf("thread fields lost: %+v", got)
	}
}

func TestBackupRoundTrip(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	app, _ := st.CreateApp(ctx, "App", "h")
	r := testReport("r1", "hashA")
	r.AppID = app.ID
	if _, err := st.IngestReport(ctx, r); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := st.Backup(dest); err != nil {
		t.Fatal(err)
	}
	if err := st.Backup(""); err == nil {
		t.Fatal("empty destination must error")
	}

	bk, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer bk.Close()
	got, err := bk.GetReport(ctx, app.ID, "r1")
	if err != nil || got.StackTraceHash != "hashA" {
		t.Fatalf("backup missing report: %+v err=%v", got, err)
	}
	if _, err := bk.GetIssue(ctx, app.ID, r.IssueID); err != nil {
		t.Fatalf("backup missing issue: %v", err)
	}
}
