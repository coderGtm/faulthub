package store

import (
	"context"
	"testing"
	"time"
)

func TestReportsPerDayFillsGaps(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	days, err := st.ReportsPerDay(ctx, appID, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 30 {
		t.Fatalf("len=%d", len(days))
	}
	today := time.Now().UTC().Format("2006-01-02")
	if days[29].Label != today || days[29].Count != 3 {
		t.Fatalf("last day: %+v", days[29])
	}
	if days[0].Count != 0 {
		t.Fatalf("first day must be zero-filled: %+v", days[0])
	}
	for i := 1; i < len(days); i++ {
		if days[i-1].Label >= days[i].Label {
			t.Fatal("days must be ascending")
		}
	}
}

func TestReportsPerDayAllApps(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	st.CreateApp(ctx, "Empty", "h2")
	all, err := st.ReportsPerDay(ctx, 0, 30)
	if err != nil {
		t.Fatal(err)
	}
	if all[29].Count != 3 {
		t.Fatalf("all apps must count 3 today: %+v", all[29])
	}
	_ = appID
}

func TestIssueReportsPerDay(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	issues, _, _ := st.ListIssues(ctx, IssueFilter{AppID: appID})
	days, err := st.IssueReportsPerDay(ctx, issues[0].ID, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 14 {
		t.Fatalf("len=%d", len(days))
	}
}

func TestBreakdown(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	byAndroid, err := st.Breakdown(ctx, appID, "android_version", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(byAndroid) != 2 || byAndroid[0].Count < byAndroid[1].Count {
		t.Fatalf("byAndroid: %+v", byAndroid)
	}
	if _, err := st.Breakdown(ctx, appID, "id", 10); err == nil {
		t.Fatal("non-whitelisted column must error")
	}
}

func TestBreakdownUnknownLabel(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	app, _ := st.CreateApp(ctx, "A", "h")
	r := testReport("rx", "h1")
	r.AppID, r.Title, r.AndroidVersion = app.ID, "t", ""
	if _, err := st.IngestReport(ctx, r); err != nil {
		t.Fatal(err)
	}
	by, _ := st.Breakdown(ctx, app.ID, "android_version", 10)
	if len(by) != 1 || by[0].Label != "(unknown)" {
		t.Fatalf("empty value must be (unknown): %+v", by)
	}
}

func TestAppStats(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	stats, err := st.AppStats(ctx, appID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Reports != 3 || stats.OpenIssues != 2 || stats.ResolvedIssues != 0 || stats.Installs != 2 {
		t.Fatalf("stats: %+v", stats)
	}
	if stats.LastReportAt.IsZero() {
		t.Fatal("LastReportAt must be set")
	}
}

func TestTopIssuesAndOverview(t *testing.T) {
	st, appID := seedIssues(t)
	ctx := context.Background()
	top, err := st.TopIssues(ctx, appID, 1)
	if err != nil || len(top) != 1 || top[0].ReportCount != 2 {
		t.Fatalf("top: %+v err=%v", top, err)
	}
	all, err := st.TopIssues(ctx, 0, 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("all-apps top: %+v err=%v", all, err)
	}
	st.CreateApp(ctx, "Empty", "h2")
	overview, err := st.AppOverview(ctx)
	if err != nil || len(overview) != 2 || overview[0].App.Name != "App" || overview[0].Reports != 3 {
		t.Fatalf("overview: %+v err=%v", overview, err)
	}
}
