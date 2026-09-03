package web

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"faulthub/internal/store"
)

func TestParseDeviceTime(t *testing.T) {
	cases := []struct {
		in      string
		wantOK  bool
		wantRFC string
	}{
		{"Thu Sep  3 09:58:00 GMT+05:30 2026", true, "2026-09-03T09:58:00+05:30"},
		{"Thu Sep 3 09:58:00 GMT+05:30 2026", true, "2026-09-03T09:58:00+05:30"},
		{"Thu Sep 13 09:58:00 GMT-04:00 2026", true, "2026-09-13T09:58:00-04:00"},
		{"Thu Sep  3 09:58:00 UTC 2026", true, "2026-09-03T09:58:00Z"},
		{"garbage", false, ""},
		{"", false, ""},
	}
	for _, tc := range cases {
		got, ok := parseDeviceTime(tc.in)
		if ok != tc.wantOK {
			t.Errorf("parseDeviceTime(%q) ok=%v want %v", tc.in, ok, tc.wantOK)
			continue
		}
		if ok {
			if rfc := got.Format("2006-01-02T15:04:05Z07:00"); rfc != tc.wantRFC {
				t.Errorf("parseDeviceTime(%q) = %q want %q", tc.in, rfc, tc.wantRFC)
			}
		}
	}
}

func TestFmtDeviceTime(t *testing.T) {
	out := string(fmtDeviceTime("Thu Sep  3 09:58:00 GMT+05:30 2026"))
	if !strings.Contains(out, `<time class="tz-device" datetime="2026-09-03T09:58:00+05:30">`) {
		t.Fatalf("missing time tag: %s", out)
	}
	if !strings.Contains(out, "Sep 3, 2026 09:58 GMT+05:30") {
		t.Fatalf("missing device text: %s", out)
	}
	if !strings.Contains(out, `class="tz-local" data-dt="2026-09-03T09:58:00+05:30"`) {
		t.Fatalf("missing local placeholder: %s", out)
	}

	if got := string(fmtDeviceTime("")); got != "—" {
		t.Fatalf("empty must be em-dash, got %q", got)
	}
	if got := string(fmtDeviceTime("not a date")); got != "not a date" {
		t.Fatalf("unparseable must pass through, got %q", got)
	}
	if got := string(fmtDeviceTime("<b>")); got != "&lt;b&gt;" {
		t.Fatalf("unparseable must be escaped, got %q", got)
	}
}

func TestHlStack(t *testing.T) {
	in := "java.lang.NullPointerException: boom\n" +
		"\tat com.example.app.MainActivity.onCreate(MainActivity.kt:42)\n" +
		"\tat android.app.Activity.onCreate(Activity.java:1)\n" +
		"Caused by: java.io.IOException: x\n" +
		"\t... 5 more"
	out := string(hlStack(in))
	for _, want := range []string{
		`<span class="tok-ex">java.lang.NullPointerException: boom</span>`,
		`<span class="tok-frame">at </span><span class="tok-fn">com.example.app.MainActivity.onCreate</span><span class="tok-loc">(MainActivity.kt:42)</span>`,
		`<span class="tok-ex">Caused by: java.io.IOException: x</span>`,
		`<span class="muted">	... 5 more</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	evil := "<script>alert(1)</script>\n\tat a.b.c(Foo.java:1)"
	out = string(hlStack(evil))
	if strings.Contains(out, "<script>") {
		t.Fatalf("must escape html:\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("escaped header missing:\n%s", out)
	}

	if got := string(hlStack("  \n ")); !strings.Contains(got, "(no stack trace)") {
		t.Fatalf("empty must show placeholder, got %q", got)
	}
}

func TestHlJSON(t *testing.T) {
	out := string(hlJSON(`{"a": "x", "n": 42, "f": 1.5, "b": true, "z": null}`))
	for _, want := range []string{
		`<span class="tok-key">&#34;a&#34;</span>`,
		`<span class="tok-str">&#34;x&#34;</span>`,
		`<span class="tok-num">42</span>`,
		`<span class="tok-num">1.5</span>`,
		`<span class="tok-bool">true</span>`,
		`<span class="tok-null">null</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Escaped quotes inside strings must not break tokenizing.
	out = string(hlJSON(`{"q": "say \"hi\""}`))
	if !strings.Contains(out, `<span class="tok-str">`) || strings.Count(out, `<span`) < 2 {
		t.Errorf("escaped-quote string broken:\n%s", out)
	}

	evil := string(hlJSON(`{"x": "<script>"}`))
	if strings.Contains(evil, "<script>") {
		t.Fatalf("must escape html:\n%s", evil)
	}

	// Invalid JSON renders escaped as-is.
	plain := string(hlJSON("{not json"))
	if strings.Contains(plain, "<span") || !strings.Contains(plain, "{not json") {
		t.Fatalf("invalid json fallback wrong: %q", plain)
	}
}

func TestReportDetailRendersHighlighting(t *testing.T) {
	srv, h := newServer(t)
	app := seedAppWithReports(t, srv)
	rep := &store.Report{
		ID: "rep-hl", AppID: app.ID,
		InstallationID: "inst-1", PackageName: "com.example.app",
		AppVersionName: "1.0", AndroidVersion: "14", PhoneModel: "Pixel 8",
		StackTrace:     "java.lang.NullPointerException: boom\n\tat com.example.app.MainActivity.onCreate(MainActivity.kt:42)",
		StackTraceHash: "hashHL", Title: "t",
		UserAppStartDate: "Thu Sep  3 09:58:00 GMT+05:30 2026",
		UserCrashDate:    "Thu Sep  3 10:00:21 GMT+05:30 2026",
		ReceivedAt:       time.Now().UTC(),
		Raw:              `{"REPORT_ID":"rep-hl","n":1}`,
	}
	if _, err := srv.Store.IngestReport(t.Context(), rep); err != nil {
		t.Fatal(err)
	}
	session, csrf := login(t, h)
	r := httptest.NewRequest("GET", "/apps/"+strconv.FormatInt(app.ID, 10)+"/reports/rep-hl", nil)
	r.AddCookie(session)
	r.AddCookie(csrf)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("report page: %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`<details class="raw-details">`,
		`<span class="tok-ex">java.lang.NullPointerException: boom</span>`,
		`<span class="tok-fn">com.example.app.MainActivity.onCreate</span>`,
		`<span class="tok-key">`,
		`Sep 3, 2026 09:58 GMT+05:30`,
		`class="tz-local" data-dt="2026-09-03T09:58:00+05:30"`,
		`class="tz-device"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("report page missing %q", want)
		}
	}
}
