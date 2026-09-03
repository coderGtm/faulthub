package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"faulthub/internal/store"
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

func TestBackupCommand(t *testing.T) {
	if err := runBackup(nil); err == nil {
		t.Fatal("missing destination must error")
	}
	dir := t.TempDir()
	t.Setenv("FAULTHUB_DATA_DIR", dir)
	t.Setenv("FAULTHUB_ADMIN_PASSWORD", "x")
	st, err := store.Open(filepath.Join(dir, "faulthub.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateApp(t.Context(), "App", "h"); err != nil {
		t.Fatal(err)
	}
	st.Close()

	dest := filepath.Join(dir, "backup.db")
	if err := runBackup([]string{dest}); err != nil {
		t.Fatal(err)
	}
	bk, err := store.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer bk.Close()
	apps, err := bk.ListApps(t.Context())
	if err != nil || len(apps) != 1 || apps[0].Name != "App" {
		t.Fatalf("backup apps: %+v err=%v", apps, err)
	}
}
