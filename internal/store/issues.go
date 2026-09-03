package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Issue struct {
	ID             int64
	AppID          int64
	AppName        string
	StackTraceHash string
	Title          string
	Status         string
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
	ReportCount    int64
	InstallCount   int64
}

type IssueFilter struct {
	AppID          int64
	Status         string
	Search         string
	AppVersion     string
	AndroidVersion string
	Device         string
	From           time.Time
	To             time.Time
	Sort           string
	Page           int
	PerPage        int
}

var reportColumns = map[string]string{
	"app_version_name": "app_version_name",
	"android_version":  "android_version",
	"phone_model":      "phone_model",
	"brand":            "brand",
}

func (s *Store) ListIssues(ctx context.Context, f IssueFilter) ([]Issue, int, error) {
	where, args := issueWhere(f)

	base := `FROM issues i
		JOIN reports r ON r.issue_id = i.id
		JOIN apps a ON a.id = i.app_id
		WHERE ` + strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM (SELECT i.id `+base+` GROUP BY i.id)`, args...).
		Scan(&total); err != nil {
		return nil, 0, err
	}

	page, perPage := f.Page, f.PerPage
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	if page < 1 {
		page = 1
	}

	order := "i.last_seen_at DESC"
	if f.Sort == "count" {
		order = "report_count DESC, i.last_seen_at DESC"
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.app_id, a.name, i.stack_trace_hash, i.title, i.status,
			i.first_seen_at, i.last_seen_at,
			COUNT(r.id) AS report_count,
			COUNT(DISTINCT r.installation_id) AS install_count
		`+base+`
		GROUP BY i.id
		ORDER BY `+order+`
		LIMIT ? OFFSET ?`, append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var issues []Issue
	for rows.Next() {
		iss, err := scanIssue(rows)
		if err != nil {
			return nil, 0, err
		}
		issues = append(issues, iss)
	}
	return issues, total, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanIssue(row scanner) (Issue, error) {
	var iss Issue
	var first, last string
	err := row.Scan(&iss.ID, &iss.AppID, &iss.AppName, &iss.StackTraceHash,
		&iss.Title, &iss.Status, &first, &last,
		&iss.ReportCount, &iss.InstallCount)
	if err != nil {
		return Issue{}, err
	}
	iss.FirstSeenAt, iss.LastSeenAt = parseTime(first), parseTime(last)
	return iss, nil
}

func issueWhere(f IssueFilter) ([]string, []any) {
	where := []string{"1=1"}
	var args []any
	if f.AppID != 0 {
		where = append(where, "i.app_id = ?")
		args = append(args, f.AppID)
	}
	if f.Status != "" {
		where = append(where, "i.status = ?")
		args = append(args, f.Status)
	}
	if f.AppVersion != "" {
		where = append(where, "r.app_version_name = ?")
		args = append(args, f.AppVersion)
	}
	if f.AndroidVersion != "" {
		where = append(where, "r.android_version = ?")
		args = append(args, f.AndroidVersion)
	}
	if f.Device != "" {
		where = append(where, "r.phone_model = ?")
		args = append(args, f.Device)
	}
	if !f.From.IsZero() {
		where = append(where, "r.received_at >= ?")
		args = append(args, fmtTime(f.From))
	}
	if !f.To.IsZero() {
		where = append(where, "r.received_at <= ?")
		args = append(args, fmtTime(f.To))
	}
	if q := f.Search; q != "" {
		where = append(where, "(i.title LIKE ? ESCAPE '\\' OR r.stack_trace LIKE ? ESCAPE '\\')")
		pat := "%" + escapeLike(q) + "%"
		args = append(args, pat, pat)
	}
	return where, args
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

func (s *Store) GetIssue(ctx context.Context, appID, issueID int64) (Issue, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT i.id, i.app_id, a.name, i.stack_trace_hash, i.title, i.status,
			i.first_seen_at, i.last_seen_at,
			(SELECT COUNT(*) FROM reports WHERE issue_id = i.id),
			(SELECT COUNT(DISTINCT installation_id) FROM reports WHERE issue_id = i.id)
		FROM issues i JOIN apps a ON a.id = i.app_id
		WHERE i.id = ? AND i.app_id = ?`, issueID, appID)
	iss, err := scanIssue(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	return iss, err
}

func (s *Store) SetIssueStatus(ctx context.Context, appID, issueID int64, status string) error {
	if status != "open" && status != "resolved" {
		return fmt.Errorf("invalid status %q", status)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE issues SET status = ? WHERE id = ? AND app_id = ?`, status, issueID, appID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteIssue(ctx context.Context, appID, issueID int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM issues WHERE id = ? AND app_id = ?`, issueID, appID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const reportCols = `id, app_id, issue_id, installation_id, package_name, app_version_code,
	app_version_name, android_version, brand, phone_model, product,
	thread_id, thread_name, thread_priority, thread_group,
	stack_trace, stack_trace_hash, user_comment, user_email,
	user_app_start_date, user_crash_date, received_at, raw`

func scanReport(row scanner) (Report, error) {
	var r Report
	var received string
	err := row.Scan(&r.ID, &r.AppID, &r.IssueID, &r.InstallationID, &r.PackageName,
		&r.AppVersionCode, &r.AppVersionName, &r.AndroidVersion, &r.Brand, &r.PhoneModel,
		&r.Product, &r.ThreadID, &r.ThreadName, &r.ThreadPriority, &r.ThreadGroup,
		&r.StackTrace, &r.StackTraceHash,
		&r.UserComment, &r.UserEmail, &r.UserAppStartDate, &r.UserCrashDate,
		&received, &r.Raw)
	if err != nil {
		return Report{}, err
	}
	r.ReceivedAt = parseTime(received)
	return r, nil
}

func (s *Store) ListIssueReports(ctx context.Context, appID, issueID int64, page, perPage int) ([]Report, int64, error) {
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	if page < 1 {
		page = 1
	}
	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reports WHERE app_id = ? AND issue_id = ?`, appID, issueID).
		Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+reportCols+` FROM reports
		WHERE app_id = ? AND issue_id = ?
		ORDER BY received_at DESC, id DESC
		LIMIT ? OFFSET ?`, appID, issueID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var reports []Report
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, 0, err
		}
		reports = append(reports, r)
	}
	return reports, total, rows.Err()
}

func (s *Store) GetReport(ctx context.Context, appID int64, id string) (Report, error) {
	r, err := scanReport(s.db.QueryRowContext(ctx,
		`SELECT `+reportCols+` FROM reports WHERE app_id = ? AND id = ?`, appID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	return r, err
}

func (s *Store) DeleteReport(ctx context.Context, appID int64, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM reports WHERE app_id = ? AND id = ?`, appID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DistinctValues(ctx context.Context, appID int64, column string) ([]string, error) {
	col, ok := reportColumns[column]
	if !ok {
		return nil, fmt.Errorf("column %q not allowed", column)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT `+col+` FROM reports
		WHERE app_id = ? AND `+col+` != '' ORDER BY 1 LIMIT 100`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vals []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	return vals, rows.Err()
}
