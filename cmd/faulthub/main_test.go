package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheckCommand(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	if err := healthcheck(ok.URL + "/healthz"); err != nil {
		t.Fatalf("healthy server must pass: %v", err)
	}
	if err := healthcheck(bad.URL + "/healthz"); err == nil {
		t.Fatal("500 must fail healthcheck")
	}
	if err := healthcheck("http://127.0.0.1:1/nope"); err == nil {
		t.Fatal("unreachable server must fail healthcheck")
	}
}

func TestHashPasswordCommand(t *testing.T) {
	out := &strings.Builder{}
	if err := hashPassword("pw", "pw", out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "$argon2id$") {
		t.Fatalf("output: %q", out.String())
	}
	if err := hashPassword("pw", "different", out); err == nil {
		t.Fatal("mismatched confirm must error")
	}
}
