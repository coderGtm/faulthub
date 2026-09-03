package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"faulthub/internal/keygen"
	"faulthub/internal/store"
)

func login(t *testing.T, h http.Handler) (session, csrf *http.Cookie) {
	t.Helper()
	w := postForm(h, "/login", url.Values{"password": {"hunter2"}})
	for _, c := range w.Result().Cookies() {
		switch c.Name {
		case "fh_session":
			session = c
		case "fh_csrf":
			csrf = c
		}
	}
	if session == nil || csrf == nil {
		t.Fatal("login failed")
	}
	return session, csrf
}

func get(h http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func seedAppWithReports(t *testing.T, srv *Server) store.App {
	t.Helper()
	key, hash, _ := keygen.NewAPIKey()
	app, err := srv.Store.CreateApp(t.Context(), "Yantra", hash)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		rep := &store.Report{
			ID: "rep-" + strconv.Itoa(i), AppID: app.ID,
			InstallationID: "inst-1", PackageName: "com.example.app",
			AppVersionName: "1.0", AndroidVersion: "14", PhoneModel: "Pixel 8",
			StackTrace:     "java.lang.NullPointerException: boom\n\tat com.example.app.MainActivity.onCreate(MainActivity.kt:4" + strconv.Itoa(i) + ")",
			StackTraceHash: "hashA", Title: "java.lang.NullPointerException: boom at com.example.app.MainActivity.onCreate",
			ReceivedAt: time.Now().UTC(),
			Raw:        `{"REPORT_ID":"rep-` + strconv.Itoa(i) + `"}`,
		}
		if _, err := srv.Store.IngestReport(t.Context(), rep); err != nil {
			t.Fatal(err)
		}
	}
	_ = key
	return app
}

func TestDashboardRenders(t *testing.T) {
	srv, h := newServer(t)
	seedAppWithReports(t, srv)
	session, csrf := login(t, h)
	w := get(h, "/", session, csrf)
	if w.Code != 200 {
		t.Fatalf("dashboard: %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Yantra", `data-kind="line"`, "Reports"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q", want)
		}
	}
}

func TestDashboardRequiresAuth(t *testing.T) {
	_, h := newServer(t)
	w := get(h, "/")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated dashboard must redirect: %d", w.Code)
	}
}

func TestAppsListAndCreate(t *testing.T) {
	srv, h := newServer(t)
	session, csrf := login(t, h)
	w := get(h, "/apps", session, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Create app") {
		t.Fatalf("apps page: %d", w.Code)
	}

	w = postForm(h, "/apps", url.Values{"name": {"Yantra"}, "csrf": {csrf.Value}}, session, csrf)
	if w.Code != 200 {
		t.Fatalf("create app: %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "fh_") || !strings.Contains(body, "shown once") {
		t.Fatal("new API key must be displayed once on create")
	}
	apps, _ := srv.Store.ListApps(t.Context())
	if len(apps) != 1 || apps[0].Name != "Yantra" {
		t.Fatalf("apps: %+v", apps)
	}

	w = postForm(h, "/apps", url.Values{"name": {""}, "csrf": {csrf.Value}}, session, csrf)
	if w.Code != 400 {
		t.Fatalf("empty name must 400: %d", w.Code)
	}
}

func TestAppDetailRenders(t *testing.T) {
	srv, h := newServer(t)
	app := seedAppWithReports(t, srv)
	session, csrf := login(t, h)
	w := get(h, "/apps/"+strconv.FormatInt(app.ID, 10), session, csrf)
	if w.Code != 200 {
		t.Fatalf("app detail: %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Yantra", `data-kind="bar"`, `data-kind="doughnut"`, "Rotate API key"} {
		if !strings.Contains(body, want) {
			t.Fatalf("app detail missing %q", want)
		}
	}
	w = get(h, "/apps/999", session, csrf)
	if w.Code != 404 {
		t.Fatalf("missing app must 404: %d", w.Code)
	}
}

func TestRotateAndDeleteApp(t *testing.T) {
	srv, h := newServer(t)
	app := seedAppWithReports(t, srv)
	session, csrf := login(t, h)
	id := strconv.FormatInt(app.ID, 10)

	w := postForm(h, "/apps/"+id+"/rotate-key", url.Values{"csrf": {csrf.Value}}, session, csrf)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "fh_") {
		t.Fatalf("rotate: %d", w.Code)
	}

	w = postForm(h, "/apps/"+id+"/delete", url.Values{"csrf": {csrf.Value}}, session, csrf)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("delete app: %d", w.Code)
	}
	if _, err := srv.Store.GetAppByID(t.Context(), app.ID); err == nil {
		t.Fatal("app must be deleted")
	}
	issues, total, _ := srv.Store.ListIssues(t.Context(), store.IssueFilter{AppID: app.ID})
	if total != 0 || len(issues) != 0 {
		t.Fatalf("app delete must cascade issues and reports: %+v", issues)
	}
}
