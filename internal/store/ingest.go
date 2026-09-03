package store

import (
	"context"
	"strconv"
	"time"
)

type Report struct {
	ID               string
	AppID            int64
	IssueID          int64
	InstallationID   string
	PackageName      string
	AppVersionCode   string
	AppVersionName   string
	AndroidVersion   string
	Brand            string
	PhoneModel       string
	Product          string
	ThreadID         int
	ThreadName       string
	ThreadPriority   int
	ThreadGroup      string
	StackTrace       string
	StackTraceHash   string
	Title            string
	UserComment      string
	UserEmail        string
	UserAppStartDate string
	UserCrashDate    string
	ReceivedAt       time.Time
	Raw              string
}

// ThreadDisplay renders the failing thread as "name (id=X, priority=Y, group=Z)",
// omitting parts that are absent. Empty when no thread info was reported.
func (r Report) ThreadDisplay() string {
	if r.ThreadName == "" && r.ThreadID == 0 && r.ThreadPriority == 0 && r.ThreadGroup == "" {
		return ""
	}
	s := r.ThreadName
	if s == "" {
		s = "(unknown)"
	}
	meta := ""
	if r.ThreadID != 0 || r.ThreadPriority != 0 || r.ThreadGroup != "" {
		parts := ""
		if r.ThreadID != 0 {
			parts = "id=" + itoa(r.ThreadID)
		}
		if r.ThreadPriority != 0 {
			if parts != "" {
				parts += ", "
			}
			parts += "priority=" + itoa(r.ThreadPriority)
		}
		if r.ThreadGroup != "" {
			if parts != "" {
				parts += ", "
			}
			parts += "group=" + r.ThreadGroup
		}
		meta = " (" + parts + ")"
	}
	return s + meta
}

func itoa(n int) string { return strconv.Itoa(n) }

func (s *Store) IngestReport(ctx context.Context, r *Report) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM reports WHERE id = ?`, r.ID).Scan(&exists); err != nil {
		return false, err
	}
	if exists > 0 {
		return false, nil
	}

	nowStr := fmtTime(r.ReceivedAt)
	var issueID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO issues (app_id, stack_trace_hash, title, status, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, 'open', ?, ?)
		ON CONFLICT (app_id, stack_trace_hash) DO UPDATE SET
			title = excluded.title,
			last_seen_at = excluded.last_seen_at,
			status = 'open'
		RETURNING id`,
		r.AppID, r.StackTraceHash, r.Title, nowStr, nowStr).Scan(&issueID)
	if err != nil {
		return false, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO reports (
			id, app_id, issue_id, installation_id, package_name, app_version_code,
			app_version_name, android_version, brand, phone_model, product,
			thread_id, thread_name, thread_priority, thread_group,
			stack_trace, stack_trace_hash,
			user_comment, user_email, user_app_start_date, user_crash_date,
			received_at, raw
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.AppID, issueID, r.InstallationID, r.PackageName, r.AppVersionCode,
		r.AppVersionName, r.AndroidVersion, r.Brand, r.PhoneModel, r.Product,
		r.ThreadID, r.ThreadName, r.ThreadPriority, r.ThreadGroup,
		r.StackTrace, r.StackTraceHash, r.UserComment,
		r.UserEmail, r.UserAppStartDate, r.UserCrashDate, nowStr, r.Raw)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.IssueID = issueID
	return true, nil
}
