package store

import (
	"context"
	"sync"
	"testing"
	"time"
)

func testReport(id, hash string) *Report {
	return &Report{
		ID:             id,
		AppID:          1,
		InstallationID: "install-1",
		PackageName:    "com.example.app",
		AppVersionName: "1.0",
		StackTrace:     "java.lang.NullPointerException: boom\n\tat com.example.app.MainActivity.onCreate(MainActivity.kt:42)",
		StackTraceHash: hash,
		Title:          "t",
		ReceivedAt:     time.Now().UTC(),
		Raw:            `{"REPORT_ID":"` + id + `"}`,
	}
}

func TestIngestReportCreatesIssue(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	app, _ := st.CreateApp(ctx, "App", "h")

	r := testReport("r1", "hashA")
	r.AppID = app.ID
	inserted, err := st.IngestReport(ctx, r)
	if err != nil || !inserted {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	if r.IssueID == 0 {
		t.Fatal("IssueID must be set")
	}
	var issueCount, reportCount int
	st.db.QueryRow("SELECT COUNT(*) FROM issues").Scan(&issueCount)
	st.db.QueryRow("SELECT COUNT(*) FROM reports").Scan(&reportCount)
	if issueCount != 1 || reportCount != 1 {
		t.Fatalf("issues=%d reports=%d", issueCount, reportCount)
	}
	var title string
	st.db.QueryRow("SELECT title FROM issues WHERE id = ?", r.IssueID).Scan(&title)
	if title != "t" {
		t.Fatalf("title = %q", title)
	}
}

func TestIngestReportDedupe(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	app, _ := st.CreateApp(ctx, "App", "h")
	r := testReport("r1", "hashA")
	r.AppID = app.ID
	if ok, err := st.IngestReport(ctx, r); !ok || err != nil {
		t.Fatal(err)
	}
	before := r.ReceivedAt
	r2 := testReport("r1", "hashA")
	r2.AppID, r2.Title, r2.ReceivedAt = app.ID, "t2", before.Add(time.Hour)
	ok, err := st.IngestReport(ctx, r2)
	if err != nil || ok {
		t.Fatalf("dup must return false: ok=%v err=%v", ok, err)
	}
	var reportCount int
	st.db.QueryRow("SELECT COUNT(*) FROM reports").Scan(&reportCount)
	if reportCount != 1 {
		t.Fatalf("dup must not add rows: %d", reportCount)
	}
	var lastSeen string
	st.db.QueryRow("SELECT last_seen_at FROM issues").Scan(&lastSeen)
	if lastSeen != fmtTime(before) {
		t.Fatalf("dup must not touch issue last_seen: %q", lastSeen)
	}
}

func TestIngestReportGroupsSameHash(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	app, _ := st.CreateApp(ctx, "App", "h")
	for _, id := range []string{"r1", "r2", "r3"} {
		r := testReport(id, "hashA")
		r.AppID = app.ID
		if _, err := st.IngestReport(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	var issueCount, reportCount, installs int
	st.db.QueryRow("SELECT COUNT(*) FROM issues").Scan(&issueCount)
	st.db.QueryRow("SELECT COUNT(*) FROM reports").Scan(&reportCount)
	st.db.QueryRow("SELECT COUNT(DISTINCT installation_id) FROM reports").Scan(&installs)
	if issueCount != 1 || reportCount != 3 || installs != 1 {
		t.Fatalf("issues=%d reports=%d installs=%d", issueCount, reportCount, installs)
	}
}

func TestIngestReportReopensResolvedIssue(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	app, _ := st.CreateApp(ctx, "App", "h")
	r := testReport("r1", "hashA")
	r.AppID = app.ID
	st.IngestReport(ctx, r)
	st.db.Exec("UPDATE issues SET status = 'resolved'")

	r2 := testReport("r2", "hashA")
	r2.AppID = app.ID
	if _, err := st.IngestReport(ctx, r2); err != nil {
		t.Fatal(err)
	}
	var status string
	st.db.QueryRow("SELECT status FROM issues").Scan(&status)
	if status != "open" {
		t.Fatalf("resolved issue must reopen on new report, got %q", status)
	}
}

func TestIngestReportConcurrentDuplicates(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	app, _ := st.CreateApp(ctx, "App", "h")
	var wg sync.WaitGroup
	results := make([]bool, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := testReport("same-id", "hashA")
			r.AppID = app.ID
			inserted, err := st.IngestReport(ctx, r)
			if err != nil {
				t.Error(err)
			}
			results[i] = inserted
		}(i)
	}
	wg.Wait()
	count := 0
	for _, ok := range results {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("exactly one insert expected, got %d", count)
	}
	var rows int
	st.db.QueryRow("SELECT COUNT(*) FROM reports").Scan(&rows)
	if rows != 1 {
		t.Fatalf("one report row expected, got %d", rows)
	}
}
