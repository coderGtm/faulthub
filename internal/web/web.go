// Package web serves the FaultHub admin dashboard.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"faulthub/internal/charts"
	"faulthub/internal/keygen"
	"faulthub/internal/ratelimit"
	"faulthub/internal/store"
)

//go:embed templates/*.html static/*
var assets embed.FS

var pageNames = []string{"login.html", "dashboard.html", "apps.html", "app_detail.html"}

var funcMap = template.FuncMap{
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.UTC().Format("Jan 2, 2006 15:04 UTC")
	},
	"chartBars": func(counts []store.Count) template.HTML {
		return charts.Bars(toBars(counts), 500, 8*36)
	},
}

const (
	sessionCookie = "fh_session"
	csrfCookie    = "fh_csrf"
)

type Server struct {
	Store        *store.Store
	LoginRate    *ratelimit.Limiter
	AdminHash    string
	CookieSecure bool
	SessionTTL   time.Duration
	TrustProxy   bool
	tpl          map[string]*template.Template
}

func New(st *store.Store, loginRate *ratelimit.Limiter, adminHash string, cookieSecure bool, sessionTTL time.Duration, trustProxy bool) (*Server, error) {
	s := &Server{
		Store: st, LoginRate: loginRate, AdminHash: adminHash,
		CookieSecure: cookieSecure, SessionTTL: sessionTTL, TrustProxy: trustProxy,
		tpl: make(map[string]*template.Template, len(pageNames)),
	}
	for _, name := range pageNames {
		t, err := template.New("layout.html").Funcs(funcMap).ParseFS(assets, "templates/layout.html", "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		s.tpl[name] = t
	}
	return s, nil
}

type baseData struct {
	Title  string
	Authed bool
	CSRF   string
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	t, ok := s.tpl[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.Execute(w, data); err != nil {
		fmt.Printf("render %s: %v\n", name, err)
	}
}

func (s *Server) csrfToken(r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil {
		return c.Value
	}
	return ""
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		hash := keygen.HashToken(c.Value)
		ok, err := s.Store.ValidSession(r.Context(), hash, time.Now())
		if err != nil || !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		expires := time.Now().Add(s.SessionTTL)
		_ = s.Store.TouchSession(r.Context(), hash, expires)
		s.setSessionCookies(w, c.Value, s.csrfToken(r), expires)
		next(w, r)
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /login", s.pageLogin)
	mux.HandleFunc("POST /login", s.submitLogin)
	mux.HandleFunc("POST /logout", s.requireAuth(s.submitLogout))
	mux.HandleFunc("GET /{$}", s.requireAuth(s.pageDashboard))
	mux.HandleFunc("GET /apps", s.requireAuth(s.pageApps))
	mux.HandleFunc("POST /apps", s.requireAuth(s.submitCreateApp))
	mux.HandleFunc("GET /apps/{id}", s.requireAuth(s.pageAppDetail))
	mux.HandleFunc("POST /apps/{id}/rotate-key", s.requireAuth(s.submitRotateKey))
	mux.HandleFunc("POST /apps/{id}/delete", s.requireAuth(s.submitDeleteApp))
	return s.secureHeaders(mux)
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) setSessionCookies(w http.ResponseWriter, session, csrf string, expires time.Time) {
	for _, c := range []*http.Cookie{
		{Name: sessionCookie, Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.CookieSecure, Expires: expires},
		{Name: csrfCookie, Value: csrf, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.CookieSecure, Expires: expires},
	} {
		http.SetCookie(w, c)
	}
}
