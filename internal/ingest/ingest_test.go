package ingest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"faulthub/internal/keygen"
	"faulthub/internal/ratelimit"
	"faulthub/internal/store"
)

const samplePayload = `{
  "REPORT_ID": "3f2d7c0e-1a2b-4c3d-9e8f-0a1b2c3d4e5f",
  "INSTALLATION_ID": "a1b2c3d4-e5f6-7890-abcd-ef0123456789",
  "PACKAGE_NAME": "com.coderGtm.yantra",
  "APP_VERSION_CODE": "123",
  "APP_VERSION_NAME": "10.5.0",
  "ANDROID_VERSION": "14",
  "BRAND": "google",
  "PHONE_MODEL": "Pixel 8",
  "PRODUCT": "shiba",
  "STACK_TRACE": "java.lang.NullPointerException: boom\n\tat android.app.Activity.onCreate(Activity.java)\n\tat com.coderGtm.yantra.MainActivity.onCreate(MainActivity.kt:42)",
  "STACK_TRACE_HASH": "9f8e7d6c5b4a39281706f5e4d3c2b1a0",
  "USER_COMMENT": "happened right after I opened settings",
  "USER_EMAIL": "user@example.com",
  "USER_APP_START_DATE": "Thu Sep  3 09:58:00 GMT+05:30 2026",
  "USER_CRASH_DATE": "Thu Sep  3 10:00:21 GMT+05:30 2026",
  "THREAD_DETAILS": {"id": 2, "name": "main", "priority": 5, "groupName": "java.lang.ThreadGroup[name=main,maxpri=10]"}
}`

const reportID = "3f2d7c0e-1a2b-4c3d-9e8f-0a1b2c3d4e5f"

func newHandler(t *testing.T) *Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "ingest.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateApp(t.Context(), "Yantra", keygen.HashToken("fh_testkey")); err != nil {
		t.Fatal(err)
	}
	return &Handler{
		Store:      st,
		Rate:       ratelimit.New(1000000, 1000000),
		MaxBody:    1 << 20,
		TrustProxy: false,
	}
}

func do(t *testing.T, h *Handler, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/v1/crash-report", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-API-Key", "fh_testkey")
	if mutate != nil {
		mutate(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestIngestSample(t *testing.T) {
	h := newHandler(t)
	w := do(t, h, samplePayload, nil)
	if w.Code != 201 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp["ok"] != true {
		t.Fatalf("body=%s", w.Body.String())
	}
	rep, err := h.Store.GetReport(t.Context(), 1, reportID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.UserEmail != "user@example.com" || rep.PhoneModel != "Pixel 8" || rep.Raw == "" || rep.ReceivedAt.IsZero() {
		t.Fatalf("stored report wrong: %+v", rep)
	}
	if rep.ThreadID != 2 || rep.ThreadName != "main" || rep.ThreadPriority != 5 ||
		rep.ThreadGroup != "java.lang.ThreadGroup[name=main,maxpri=10]" {
		t.Fatalf("thread fields wrong: %+v", rep)
	}
	if got := rep.ThreadDisplay(); got != "main (id=2, priority=5, group=java.lang.ThreadGroup[name=main,maxpri=10])" {
		t.Fatalf("thread display = %q", got)
	}
	if !strings.Contains(rep.Raw, reportID) {
		t.Fatal("raw must be the original body")
	}
	issues, total, _ := h.Store.ListIssues(t.Context(), store.IssueFilter{AppID: 1})
	if total != 1 {
		t.Fatalf("issues: %+v", issues)
	}
	want := "java.lang.NullPointerException: boom at com.coderGtm.yantra.MainActivity.onCreate(MainActivity.kt:42)"
	if issues[0].Title != want {
		t.Fatalf("title = %q want %q", issues[0].Title, want)
	}
}

func TestIngestDuplicate(t *testing.T) {
	h := newHandler(t)
	if w := do(t, h, samplePayload, nil); w.Code != 201 {
		t.Fatalf("first: %d", w.Code)
	}
	if w := do(t, h, samplePayload, nil); w.Code != 200 {
		t.Fatalf("dup must be 200, got %d", w.Code)
	}
	issues, total, _ := h.Store.ListIssues(t.Context(), store.IssueFilter{AppID: 1})
	if total != 1 || issues[0].ReportCount != 1 {
		t.Fatalf("dup must not add rows: total=%d", total)
	}
}

func TestIngestAuth(t *testing.T) {
	h := newHandler(t)
	if w := do(t, h, samplePayload, func(r *http.Request) { r.Header.Del("X-API-Key") }); w.Code != 401 {
		t.Fatalf("missing key: %d", w.Code)
	}
	if w := do(t, h, samplePayload, func(r *http.Request) { r.Header.Set("X-API-Key", "fh_wrong") }); w.Code != 401 {
		t.Fatalf("wrong key: %d", w.Code)
	}
	if _, err := h.Store.GetReport(t.Context(), 1, reportID); err == nil {
		t.Fatal("401 must not persist")
	}
}

func TestIngestValidation(t *testing.T) {
	h := newHandler(t)
	noID := strings.Replace(samplePayload, `"REPORT_ID": "`+reportID+`",`, "", 1)
	if w := do(t, h, noID, nil); w.Code != 400 {
		t.Fatalf("missing REPORT_ID: %d", w.Code)
	}
	longID := strings.Replace(samplePayload, reportID, strings.Repeat("x", 129), 1)
	if w := do(t, h, longID, nil); w.Code != 400 {
		t.Fatalf("long REPORT_ID: %d", w.Code)
	}
	if w := do(t, h, "{not json", nil); w.Code != 400 {
		t.Fatalf("malformed: %d", w.Code)
	}
	if w := do(t, h, "[]", nil); w.Code != 400 {
		t.Fatalf("non-object body: %d", w.Code)
	}
	if w := do(t, h, "null", nil); w.Code != 400 {
		t.Fatalf("null body: %d", w.Code)
	}
	if w := do(t, h, samplePayload, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }); w.Code != 415 {
		t.Fatalf("content type: %d", w.Code)
	}
	if w := do(t, h, samplePayload, func(r *http.Request) { r.Header.Del("Content-Type") }); w.Code != 415 {
		t.Fatalf("missing content type: %d", w.Code)
	}
	if w := do(t, h, samplePayload, func(r *http.Request) {
		r.Header.Set("Content-Type", "application/json; charset=utf-8")
	}); w.Code != 201 {
		t.Fatalf("charset param must be tolerated: %d", w.Code)
	}
}

func TestIngestUnknownKeysIgnored(t *testing.T) {
	h := newHandler(t)
	body := strings.Replace(samplePayload, "{", `{"SOME_FUTURE_FIELD":123,"ANOTHER":null,`, 1)
	if w := do(t, h, body, nil); w.Code != 201 {
		t.Fatalf("unknown keys must be ignored: %d body=%s", w.Code, w.Body.String())
	}
}

func TestIngestNonStringScalarsStringified(t *testing.T) {
	h := newHandler(t)
	body := strings.Replace(samplePayload, `"REPORT_ID": "`+reportID+`"`, `"REPORT_ID": 12345`, 1)
	if w := do(t, h, body, nil); w.Code != 201 {
		t.Fatalf("numeric REPORT_ID accepted defensively: %d", w.Code)
	}
	if _, err := h.Store.GetReport(t.Context(), 1, "12345"); err != nil {
		t.Fatal("stringified id must be stored")
	}
}

func TestIngestNullValuesTolerated(t *testing.T) {
	h := newHandler(t)
	body := strings.Replace(samplePayload, `"USER_COMMENT": "happened right after I opened settings",`, `"USER_COMMENT": null,`, 1)
	if w := do(t, h, body, nil); w.Code != 201 {
		t.Fatalf("null value: %d", w.Code)
	}
	rep, _ := h.Store.GetReport(t.Context(), 1, reportID)
	if rep.UserComment != "" {
		t.Fatalf("null must store empty: %q", rep.UserComment)
	}
}

func TestIngestNoStackTrace(t *testing.T) {
	h := newHandler(t)
	body := strings.Replace(samplePayload,
		`"STACK_TRACE": "java.lang.NullPointerException: boom\n\tat android.app.Activity.onCreate(Activity.java)\n\tat com.coderGtm.yantra.MainActivity.onCreate(MainActivity.kt:42)",`,
		`"STACK_TRACE": null,`, 1)
	body = strings.Replace(body, `"STACK_TRACE_HASH": "9f8e7d6c5b4a39281706f5e4d3c2b1a0",`, "", 1)
	if w := do(t, h, body, nil); w.Code != 201 {
		t.Fatalf("no stack trace must be accepted: %d body=%s", w.Code, w.Body.String())
	}
	issues, total, _ := h.Store.ListIssues(t.Context(), store.IssueFilter{AppID: 1})
	if total != 1 || issues[0].Title != "(no stack trace)" {
		t.Fatalf("issues: %+v", issues)
	}
}

func TestIngestHashTooLong(t *testing.T) {
	h := newHandler(t)
	body := strings.Replace(samplePayload,
		`"STACK_TRACE_HASH": "9f8e7d6c5b4a39281706f5e4d3c2b1a0"`,
		`"STACK_TRACE_HASH": "`+strings.Repeat("a", 129)+`"`, 1)
	if w := do(t, h, body, nil); w.Code != 400 {
		t.Fatalf("over-long hash: %d", w.Code)
	}
}

func TestIngestTooDeeplyNested(t *testing.T) {
	h := newHandler(t)
	deep := strings.Repeat("[", 40) + strings.Repeat("]", 40)
	body := `{"REPORT_ID": "deep-1", "THREAD_DETAILS": ` + deep + `}`
	if w := do(t, h, body, nil); w.Code != 400 {
		t.Fatalf("deep nesting: %d", w.Code)
	}
	if _, err := h.Store.GetReport(t.Context(), 1, "deep-1"); err == nil {
		t.Fatal("rejected report must not persist")
	}
	// Sanity: a legit nested THREAD_DETAILS object still passes.
	if w := do(t, h, samplePayload, nil); w.Code != 201 {
		t.Fatalf("normal nested object: %d", w.Code)
	}
}

func TestIngestThreadDetails(t *testing.T) {
	h := newHandler(t)

	// Missing THREAD_DETAILS → zeros.
	body := strings.Replace(samplePayload,
		",\n  \"THREAD_DETAILS\": {\"id\": 2, \"name\": \"main\", \"priority\": 5, \"groupName\": \"java.lang.ThreadGroup[name=main,maxpri=10]\"}",
		``, 1)
	body = strings.Replace(body, reportID, "thread-missing", 1)
	if w := do(t, h, body, nil); w.Code != 201 {
		t.Fatalf("missing thread: %d body=%s", w.Code, w.Body.String())
	}
	rep, _ := h.Store.GetReport(t.Context(), 1, "thread-missing")
	if rep.ThreadID != 0 || rep.ThreadName != "" || rep.ThreadPriority != 0 || rep.ThreadGroup != "" {
		t.Fatalf("missing thread must store zeros: %+v", rep)
	}
	if rep.ThreadDisplay() != "" {
		t.Fatalf("empty thread display must be empty: %q", rep.ThreadDisplay())
	}

	// Null THREAD_DETAILS → zeros.
	body = strings.Replace(samplePayload,
		`"THREAD_DETAILS": {"id": 2, "name": "main", "priority": 5, "groupName": "java.lang.ThreadGroup[name=main,maxpri=10]"}`,
		`"THREAD_DETAILS": null`, 1)
	body = strings.Replace(body, reportID, "thread-null", 1)
	if w := do(t, h, body, nil); w.Code != 201 {
		t.Fatalf("null thread: %d", w.Code)
	}
	rep, _ = h.Store.GetReport(t.Context(), 1, "thread-null")
	if rep.ThreadName != "" || rep.ThreadID != 0 {
		t.Fatalf("null thread must store zeros: %+v", rep)
	}

	// Partial object → only present fields.
	body = strings.Replace(samplePayload,
		`"THREAD_DETAILS": {"id": 2, "name": "main", "priority": 5, "groupName": "java.lang.ThreadGroup[name=main,maxpri=10]"}`,
		`"THREAD_DETAILS": {"name": "Binder:123_4"}`, 1)
	body = strings.Replace(body, reportID, "thread-partial", 1)
	if w := do(t, h, body, nil); w.Code != 201 {
		t.Fatalf("partial thread: %d", w.Code)
	}
	rep, _ = h.Store.GetReport(t.Context(), 1, "thread-partial")
	if rep.ThreadName != "Binder:123_4" || rep.ThreadID != 0 || rep.ThreadPriority != 0 || rep.ThreadGroup != "" {
		t.Fatalf("partial thread wrong: %+v", rep)
	}
}

func TestIngestOversizedBody(t *testing.T) {
	h := newHandler(t)
	big := `{"pad":"` + strings.Repeat("a", 2<<20) + `"}`
	if w := do(t, h, big, nil); w.Code != 413 {
		t.Fatalf("oversized: %d", w.Code)
	}
}

func TestIngestRateLimited(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "rl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.CreateApp(t.Context(), "A", keygen.HashToken("fh_testkey"))
	n := time.Unix(5000, 0)
	h := &Handler{
		Store:      st,
		Rate:       ratelimit.NewWithClock(60, 2, func() time.Time { return n }),
		MaxBody:    1 << 20,
		TrustProxy: false,
	}
	var codes []int
	for i := 0; i < 4; i++ {
		body := strings.Replace(samplePayload, reportID, fmt.Sprintf("rl-%d", i), 1)
		w := do(t, h, body, nil)
		codes = append(codes, w.Code)
	}
	if codes[0] != 201 || codes[1] != 201 || codes[2] != 429 || codes[3] != 429 {
		t.Fatalf("codes = %v", codes)
	}
	last := do(t, h, samplePayload, nil)
	if last.Header().Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
}

func TestIngestConcurrentDuplicates(t *testing.T) {
	h := newHandler(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := do(t, h, samplePayload, nil)
			mu.Lock()
			codes[w.Code]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes[201] != 1 || codes[200] != 3 {
		t.Fatalf("codes = %v", codes)
	}
}

func TestTitleFromTrace(t *testing.T) {
	cases := []struct {
		name, trace, pkg, want string
	}{
		{
			"app frame preferred",
			"java.lang.NullPointerException: x\n\tat android.app.Activity.onCreate(Activity.java)\n\tat com.a.b.MainActivity.onCreate(MainActivity.kt:42)",
			"com.a.b",
			"java.lang.NullPointerException: x at com.a.b.MainActivity.onCreate(MainActivity.kt:42)",
		},
		{
			"fallback first frame",
			"java.lang.IllegalStateException: y\n\tat foo.Bar.baz(Bar.java:1)",
			"com.a.b",
			"java.lang.IllegalStateException: y at foo.Bar.baz(Bar.java:1)",
		},
		{"no trace", "", "com.a.b", "(no stack trace)"},
		{"whitespace trace", "  \n ", "com.a.b", "(no stack trace)"},
	}
	for _, tc := range cases {
		if got := TitleFromTrace(tc.trace, tc.pkg); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	long := TitleFromTrace(strings.Repeat("x", 500)+"\n\tat a.b.c(a.java:1)", "")
	if len([]rune(long)) > 300 {
		t.Fatalf("title must truncate to 300 runes, got %d", len([]rune(long)))
	}
}
