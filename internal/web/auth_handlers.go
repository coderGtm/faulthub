package web

import (
	"crypto/subtle"
	"net/http"
	"time"

	"faulthub/internal/auth"
	"faulthub/internal/keygen"
	"faulthub/internal/realip"
)

type loginData struct {
	baseData
	Error string
}

func (s *Server) pageLogin(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if ok, _ := s.Store.ValidSession(r.Context(), keygen.HashToken(c.Value), time.Now()); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	s.render(w, http.StatusOK, "login.html", loginData{baseData: baseData{Title: "Log in"}})
}

func (s *Server) submitLogin(w http.ResponseWriter, r *http.Request) {
	ip := realip.Client(r, s.TrustProxy)
	if ok, _ := s.LoginRate.Allow(ip); !ok {
		s.render(w, http.StatusTooManyRequests, "login.html",
			loginData{baseData: baseData{Title: "Log in"}, Error: "Too many attempts. Try again later."})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	password := r.PostFormValue("password")
	if !auth.VerifyPassword(s.AdminHash, password) {
		s.render(w, http.StatusUnauthorized, "login.html",
			loginData{baseData: baseData{Title: "Log in"}, Error: "Wrong password."})
		return
	}
	sessionToken, sessionHash, err := keygen.NewToken()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	csrfToken, _, err := keygen.NewToken()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(s.SessionTTL)
	if err := s.Store.CreateSession(r.Context(), sessionHash, expires); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	s.setSessionCookies(w, sessionToken, csrfToken, expires)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) submitLogout(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.Store.DeleteSession(r.Context(), keygen.HashToken(c.Value))
	}
	s.clearSessionCookies(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) checkCSRF(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil {
		http.Error(w, "missing csrf cookie", http.StatusBadRequest)
		return false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.PostFormValue("csrf"))) != 1 {
		http.Error(w, "csrf mismatch", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) clearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", HttpOnly: true,
			SameSite: http.SameSiteLaxMode, Secure: s.CookieSecure,
			MaxAge: -1,
		})
	}
}
