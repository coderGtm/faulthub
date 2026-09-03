package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"faulthub/internal/store"
)

func issuePath(app store.App, suffix string) string {
	return "/apps/" + strconv.FormatInt(app.ID, 10) + "/issues" + suffix
}

func TestIssuesListFiltersAndPaginates(t *testing.T) {
	srv, h := newServer(t)
	app := seedAppWithReports(t, srv)
	session, csrf := login(t, h)

	w := get(h, issuePath(app, ""), session, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "java.lang.NullPointerException") {
		t.Fatalf("issues list: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "3") {
		t.Fatal("report count 3 expected")
	}

	w = get(h, issuePath(app, "?status=resolved"), session, csrf)
	if w.Code != 200 || strings.Contains(w.Body.String(), "java.lang.NullPointerException") {
		t.Fatalf("resolved filter must hide open issues: %d", w.Code)
	}

	w = get(h, issuePath(app, "?q=nonexistentsearchterm"), session, csrf)
	if w.Code != 200 || strings.Contains(w.Body.String(), "java.lang.NullPointerException") {
		t.Fatal("search with no hits must show empty state")
	}

	w = get(h, issuePath(app, "?ver=1.0&os=14&device=Pixel+8"), session, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "java.lang.NullPointerException") {
		t.Fatalf("report-level filters: %d", w.Code)
	}

	w = get(h, issuePath(app, "?from=01-01-2000&to=01-01-2001"), session, csrf)
	if w.Code != 200 || strings.Contains(w.Body.String(), "java.lang.NullPointerException") {
		t.Fatal("dd-mm-yyyy range outside data must show empty state")
	}
	w = get(h, issuePath(app, "?from=01-01-2000&to=01-01-2100"), session, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "java.lang.NullPointerException") {
		t.Fatal("dd-mm-yyyy range covering data must list issues")
	}
}

func TestParseFilterDate(t *testing.T) {
	if got, ok := parseFilterDate("03-09-2026"); !ok || got.Day() != 3 || got.Month() != 9 || got.Year() != 2026 {
		t.Fatalf("dd-mm-yyyy: %+v %v", got, ok)
	}
	if _, ok := parseFilterDate("2026-09-03"); !ok {
		t.Fatal("iso date must keep working")
	}
	if _, ok := parseFilterDate("not-a-date"); ok {
		t.Fatal("garbage must not parse")
	}
}

func TestIssueDetailAndStatus(t *testing.T) {
	srv, h := newServer(t)
	app := seedAppWithReports(t, srv)
	session, csrf := login(t, h)

	issues, _, _ := srv.Store.ListIssues(t.Context(), store.IssueFilter{AppID: app.ID})
	path := issuePath(app, "/"+strconv.FormatInt(issues[0].ID, 10))

	w := get(h, path, session, csrf)
	if w.Code != 200 {
		t.Fatalf("issue detail: %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"java.lang.NullPointerException", "Pixel 8", "report"} {
		if !strings.Contains(body, want) {
			t.Fatalf("issue detail missing %q", want)
		}
	}

	w = postForm(h, path+"/status", url.Values{"status": {"resolved"}, "csrf": {csrf.Value}}, session, csrf)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status change: %d", w.Code)
	}
	got, _ := srv.Store.GetIssue(t.Context(), app.ID, issues[0].ID)
	if got.Status != "resolved" {
		t.Fatalf("status = %q", got.Status)
	}

	w = postForm(h, path+"/status", url.Values{"status": {"bogus"}, "csrf": {csrf.Value}}, session, csrf)
	if w.Code != 400 {
		t.Fatalf("invalid status: %d", w.Code)
	}
}

func TestIssueAndReportDelete(t *testing.T) {
	srv, h := newServer(t)
	app := seedAppWithReports(t, srv)
	session, csrf := login(t, h)

	issues, _, _ := srv.Store.ListIssues(t.Context(), store.IssueFilter{AppID: app.ID})
	issueID := strconv.FormatInt(issues[0].ID, 10)
	reports, _, _ := srv.Store.ListIssueReports(t.Context(), app.ID, issues[0].ID, 1, 25)
	repPath := "/apps/" + strconv.FormatInt(app.ID, 10) + "/reports/" + reports[0].ID

	w := get(h, repPath, session, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "rep-") {
		t.Fatalf("report detail: %d", w.Code)
	}

	w = postForm(h, repPath+"/delete", url.Values{"csrf": {csrf.Value}}, session, csrf)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("delete report: %d", w.Code)
	}
	if _, err := srv.Store.GetReport(t.Context(), app.ID, reports[0].ID); err == nil {
		t.Fatal("report must be deleted (email purged with it)")
	}

	w = postForm(h, issuePath(app, "/"+issueID)+"/delete", url.Values{"csrf": {csrf.Value}}, session, csrf)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("delete issue: %d", w.Code)
	}
	if _, err := srv.Store.GetIssue(t.Context(), app.ID, issues[0].ID); err == nil {
		t.Fatal("issue must be deleted")
	}
}

func TestReportLinksBackToIssue(t *testing.T) {
	srv, h := newServer(t)
	app := seedAppWithReports(t, srv)
	session, csrf := login(t, h)
	issues, _, _ := srv.Store.ListIssues(t.Context(), store.IssueFilter{AppID: app.ID})
	reports, _, _ := srv.Store.ListIssueReports(t.Context(), app.ID, issues[0].ID, 1, 25)
	path := "/apps/" + strconv.FormatInt(app.ID, 10) + "/reports/" + reports[0].ID
	w := get(h, path, session, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), strconv.FormatInt(issues[0].ID, 10)) {
		t.Fatalf("report page must link to its issue: %d", w.Code)
	}
}
