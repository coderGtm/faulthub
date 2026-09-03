package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func seedIssues(t *testing.T) (*Store, int64) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	app, _ := st.CreateApp(ctx, "App", "h")
	for i, hash := range []string{"hashA", "hashA", "hashB"} {
		r := testReport("r"+string(rune('1'+i)), hash)
		r.AppID, r.Title = app.ID, "title "+hash
		r.AndroidVersion = "14"
		r.AppVersionName = "1.1"
		r.PhoneModel = "Pixel 8"
		r.InstallationID = "install-" + hash
		if hash == "hashB" {
			r.AndroidVersion = "13"
			r.PhoneModel = "Galaxy S23"
		}
		if _, err := st.IngestReport(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	return st, app.ID
}

func TestListIssuesFiltersAndCounts(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()

	issues, total, err := st.ListIssues(ctx, IssueFilter{AppID: appID})
	if err != nil || total != 2 || len(issues) != 2 {
		t.Fatalf("total=%d len=%d err=%v", total, len(issues), err)
	}
	for _, iss := range issues {
		if iss.AppName != "App" || iss.Status != "open" {
			t.Fatalf("issue fields wrong: %+v", iss)
		}
	}

	issues, total, _ = st.ListIssues(ctx, IssueFilter{AppID: appID, AndroidVersion: "14"})
	if total != 1 || issues[0].ReportCount != 2 {
		t.Fatalf("android filter: total=%d %+v", total, issues)
	}

	issues, total, _ = st.ListIssues(ctx, IssueFilter{AppID: appID, Device: "Galaxy S23"})
	if total != 1 || issues[0].ReportCount != 1 {
		t.Fatalf("device filter: total=%d %+v", total, issues)
	}

	_, total, _ = st.ListIssues(ctx, IssueFilter{AppID: appID, Search: "hashB"})
	if total != 1 {
		t.Fatalf("search on title: total=%d", total)
	}

	issues, total, _ = st.ListIssues(ctx, IssueFilter{AppID: appID, Sort: "count"})
	if total != 2 || issues[0].ReportCount < issues[1].ReportCount {
		t.Fatalf("count sort: %+v", issues)
	}

	issues, _, _ = st.ListIssues(ctx, IssueFilter{AppID: appID, Page: 2, PerPage: 1})
	if len(issues) != 1 {
		t.Fatalf("pagination: len=%d", len(issues))
	}

	_, total, _ = st.ListIssues(ctx, IssueFilter{AppID: appID, Status: "resolved"})
	if total != 0 {
		t.Fatalf("status filter: total=%d", total)
	}
}

func TestIssueLifecycle(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	issues, _, _ := st.ListIssues(ctx, IssueFilter{AppID: appID})
	first := issues[0]

	if err := st.SetIssueStatus(ctx, appID, first.ID, "resolved"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetIssue(ctx, appID, first.ID)
	if err != nil || got.Status != "resolved" || got.AppName != "App" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if err := st.SetIssueStatus(ctx, appID, first.ID, "bogus"); err == nil {
		t.Fatal("invalid status must error")
	}
	if _, err := st.GetIssue(ctx, appID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatal("want ErrNotFound")
	}

	reports, total, err := st.ListIssueReports(ctx, appID, first.ID, 1, 25)
	if err != nil || int64(len(reports)) != total {
		t.Fatalf("total=%d err=%v", total, err)
	}
	if len(reports) > 0 {
		rid := reports[0].ID
		full, err := st.GetReport(ctx, appID, rid)
		if err != nil || full.ID != rid || full.Raw == "" || full.ReceivedAt.IsZero() {
			t.Fatalf("GetReport: %+v err=%v", full, err)
		}
		if err := st.DeleteReport(ctx, appID, rid); err != nil {
			t.Fatal(err)
		}
		if _, err := st.GetReport(ctx, appID, rid); !errors.Is(err, ErrNotFound) {
			t.Fatal("report must be gone")
		}
	}

	if err := st.DeleteIssue(ctx, appID, first.ID); err != nil {
		t.Fatal(err)
	}
	var rows int
	st.db.QueryRow("SELECT COUNT(*) FROM reports WHERE issue_id = ?", first.ID).Scan(&rows)
	if rows != 0 {
		t.Fatal("issue delete must cascade reports")
	}
	if err := st.DeleteIssue(ctx, appID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleting missing issue must be ErrNotFound")
	}
}

func TestDistinctValues(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	vers, err := st.DistinctValues(ctx, appID, "android_version")
	if err != nil || len(vers) != 2 {
		t.Fatalf("versions: %v %v", vers, err)
	}
	if _, err := st.DistinctValues(ctx, appID, "stack_trace"); err == nil {
		t.Fatal("non-whitelisted column must error")
	}
}

func TestDateRangeFilter(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	_, total, _ := st.ListIssues(ctx, IssueFilter{AppID: appID, From: time.Now().UTC().Add(-time.Hour)})
	if total != 2 {
		t.Fatalf("from-filter: total=%d", total)
	}
	_, total, _ = st.ListIssues(ctx, IssueFilter{AppID: appID, From: time.Now().UTC().Add(time.Hour)})
	if total != 0 {
		t.Fatalf("future from-filter: total=%d", total)
	}
}

func TestSearchEscapesLikeWildcards(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	_, total, _ := st.ListIssues(ctx, IssueFilter{AppID: appID, Search: "title hashA"})
	if total != 1 {
		t.Fatalf("plain search: total=%d", total)
	}
}
