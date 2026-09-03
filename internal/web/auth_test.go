package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"faulthub/internal/auth"
	"faulthub/internal/keygen"
	"faulthub/internal/ratelimit"
	"faulthub/internal/store"
)

func newServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hash, err := auth.HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(st, ratelimit.New(5, 3), hash, false, time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	return srv, srv.Routes()
}

func postForm(h http.Handler, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestLoginPageRenders(t *testing.T) {
	_, h := newServer(t)
	r := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "FaultHub") {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestLoginFlow(t *testing.T) {
	_, h := newServer(t)
	w := postForm(h, "/login", url.Values{"password": {"hunter2"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("login: %d", w.Code)
	}
	var session, csrf *http.Cookie
	for _, c := range w.Result().Cookies() {
		switch c.Name {
		case "fh_session":
			session = c
		case "fh_csrf":
			csrf = c
		}
	}
	if session == nil || csrf == nil {
		t.Fatal("session and csrf cookies must be set")
	}
	if !session.HttpOnly || session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie flags wrong: %+v", session)
	}

	r := httptest.NewRequest("GET", "/login", nil)
	r.AddCookie(session)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("authed user hitting /login must redirect: %d", w.Code)
	}

	w = postForm(h, "/logout", url.Values{"csrf": {csrf.Value}}, session, csrf)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("logout: %d", w.Code)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	_, h := newServer(t)
	w := postForm(h, "/login", url.Values{"password": {"wrong"}})
	if w.Code != 401 || !strings.Contains(w.Body.String(), "Wrong password") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLoginRateLimited(t *testing.T) {
	_, h := newServer(t)
	for i := 0; i < 3; i++ {
		if w := postForm(h, "/login", url.Values{"password": {"wrong"}}); w.Code != 401 {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	if w := postForm(h, "/login", url.Values{"password": {"hunter2"}}); w.Code != 429 {
		t.Fatalf("4th login attempt must be rate limited: %d", w.Code)
	}
}

func TestCSRFRequired(t *testing.T) {
	srv, h := newServer(t)
	token, hash, _ := keygen.NewToken()
	srv.Store.CreateSession(t.Context(), hash, time.Now().Add(time.Hour))
	session := &http.Cookie{Name: "fh_session", Value: token}
	csrf := &http.Cookie{Name: "fh_csrf", Value: "csrfval"}

	w := postForm(h, "/logout", url.Values{"csrf": {"wrong"}}, session, csrf)
	if w.Code != 400 {
		t.Fatalf("csrf mismatch: %d", w.Code)
	}
}

func TestLogoutRequiresAuth(t *testing.T) {
	_, h := newServer(t)
	w := postForm(h, "/logout", url.Values{"csrf": {"x"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated logout must redirect: %d", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	_, h := newServer(t)
	r := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	hdr := w.Header()
	if hdr.Get("Content-Security-Policy") == "" || hdr.Get("X-Frame-Options") != "DENY" ||
		hdr.Get("X-Content-Type-Options") != "nosniff" || hdr.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("headers: %v", hdr)
	}
}

func TestStaticServed(t *testing.T) {
	_, h := newServer(t)
	for _, path := range []string{"/static/app.css", "/static/app.js"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/static/../go.mod", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("path traversal must not serve files outside static")
	}
}
