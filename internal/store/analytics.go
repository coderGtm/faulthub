package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Count struct {
	Label string
	Count int64
}

type AppStats struct {
	Reports        int64
	OpenIssues     int64
	ResolvedIssues int64
	Installs       int64
	LastReportAt   time.Time
}

type AppOverview struct {
	App          App
	Reports      int64
	OpenIssues   int64
	LastReportAt time.Time
}

func (s *Store) ReportsPerDay(ctx context.Context, appID int64, days int) ([]Count, error) {
	return s.perDay(ctx, "app_id", appID, days)
}

func (s *Store) IssueReportsPerDay(ctx context.Context, issueID int64, days int) ([]Count, error) {
	return s.perDay(ctx, "issue_id", issueID, days)
}

func (s *Store) perDay(ctx context.Context, col string, id int64, days int) ([]Count, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour)
	where := fmt.Sprintf("WHERE %s = ? AND received_at >= ?", col)
	args := []any{id, cutoff.Format(time.RFC3339)}
	if id == 0 {
		where = "WHERE received_at >= ?"
		args = []any{cutoff.Format(time.RFC3339)}
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT substr(received_at, 1, 10) AS day, COUNT(*) FROM reports `+where+`
		GROUP BY day ORDER BY day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDay := map[string]int64{}
	for rows.Next() {
		var d string
		var n int64
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		byDay[d] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Count, 0, days)
	for i := 0; i < days; i++ {
		day := cutoff.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, Count{Label: day, Count: byDay[day]})
	}
	return out, nil
}

func (s *Store) Breakdown(ctx context.Context, appID int64, column string, limit int) ([]Count, error) {
	col, ok := reportColumns[column]
	if !ok {
		return nil, fmt.Errorf("column %q not allowed", column)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+col+`, COUNT(*) AS c FROM reports
		WHERE app_id = ? GROUP BY `+col+` ORDER BY c DESC, `+col+` LIMIT ?`, appID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Count
	for rows.Next() {
		var label string
		var n int64
		if err := rows.Scan(&label, &n); err != nil {
			return nil, err
		}
		if label == "" {
			label = "(unknown)"
		}
		out = append(out, Count{Label: label, Count: n})
	}
	return out, rows.Err()
}

func (s *Store) AppStats(ctx context.Context, appID int64) (AppStats, error) {
	var st AppStats
	var last sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM reports WHERE app_id = ?1),
			(SELECT COUNT(*) FROM issues WHERE app_id = ?1 AND status = 'open'),
			(SELECT COUNT(*) FROM issues WHERE app_id = ?1 AND status = 'resolved'),
			(SELECT COUNT(DISTINCT installation_id) FROM reports WHERE app_id = ?1),
			(SELECT MAX(received_at) FROM reports WHERE app_id = ?1)`, appID).
		Scan(&st.Reports, &st.OpenIssues, &st.ResolvedIssues, &st.Installs, &last)
	if err != nil {
		return AppStats{}, err
	}
	if last.Valid {
		st.LastReportAt = parseTime(last.String)
	}
	return st, nil
}

func (s *Store) TopIssues(ctx context.Context, appID int64, limit int) ([]Issue, error) {
	where, args := issueWhere(IssueFilter{AppID: appID})
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.app_id, a.name, i.stack_trace_hash, i.title, i.status,
			i.first_seen_at, i.last_seen_at,
			COUNT(r.id), COUNT(DISTINCT r.installation_id)
		FROM issues i
		JOIN reports r ON r.issue_id = i.id
		JOIN apps a ON a.id = i.app_id
		WHERE `+strings.Join(where, " AND ")+`
		GROUP BY i.id
		ORDER BY i.last_seen_at DESC, i.id
		LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var issues []Issue
	for rows.Next() {
		iss, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, iss)
	}
	return issues, rows.Err()
}

func (s *Store) AppOverview(ctx context.Context) ([]AppOverview, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.name, a.api_key_hash, a.created_at,
			(SELECT COUNT(*) FROM reports WHERE app_id = a.id),
			(SELECT COUNT(*) FROM issues WHERE app_id = a.id AND status = 'open'),
			(SELECT MAX(received_at) FROM reports WHERE app_id = a.id)
		FROM apps a ORDER BY a.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppOverview
	for rows.Next() {
		var o AppOverview
		var created string
		var last sql.NullString
		if err := rows.Scan(&o.App.ID, &o.App.Name, &o.App.APIKeyHash, &created,
			&o.Reports, &o.OpenIssues, &last); err != nil {
			return nil, err
		}
		o.App.CreatedAt = parseTime(created)
		if last.Valid {
			o.LastReportAt = parseTime(last.String)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
