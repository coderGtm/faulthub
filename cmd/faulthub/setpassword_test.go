package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeEscape(t *testing.T) {
	got := composeEscape("$argon2id$v=19$m=19456,t=2,p=1$abc$def")
	want := "$$argon2id$$v=19$$m=19456,t=2,p=1$$abc$$def"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if composeEscape("no dollars") != "no dollars" {
		t.Fatal("no-dollar input must pass through unchanged")
	}
}

func TestRewriteEnvReplacesHash(t *testing.T) {
	content := "# comment\nFAULTHUB_PORT=8080\nFAULTHUB_ADMIN_PASSWORD_HASH=$$old$$\nFAULTHUB_COOKIE_SECURE=true\n"
	got := rewriteEnv(content, "$$argon2id$$v=19")
	want := "# comment\nFAULTHUB_PORT=8080\nFAULTHUB_ADMIN_PASSWORD_HASH=$$argon2id$$v=19\nFAULTHUB_COOKIE_SECURE=true\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRewriteEnvAppendsWhenMissing(t *testing.T) {
	got := rewriteEnv("FAULTHUB_PORT=8080\n", "$$new$$")
	want := "FAULTHUB_PORT=8080\nFAULTHUB_ADMIN_PASSWORD_HASH=$$new$$\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRewriteEnvDropsPlaintextPassword(t *testing.T) {
	content := "FAULTHUB_ADMIN_PASSWORD=secret\nFAULTHUB_PORT=8080\nFAULTHUB_ADMIN_PASSWORD_HASH=$$old$$\n"
	got := rewriteEnv(content, "$$new$$")
	if strings.Contains(got, "FAULTHUB_ADMIN_PASSWORD=") {
		t.Fatalf("plaintext line must be removed:\n%q", got)
	}
	if !strings.Contains(got, "FAULTHUB_ADMIN_PASSWORD_HASH=$$new$$") {
		t.Fatalf("hash line missing:\n%q", got)
	}
}

func TestWriteEnvHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := writeEnvHash(path, "$argon2id$v=19$m=19456,t=2,p=1$abc$def"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "FAULTHUB_ADMIN_PASSWORD_HASH=$$argon2id$$v=19$$m=19456,t=2,p=1$$abc$$def\n"
	if string(content) != want {
		t.Fatalf("got %q want %q", string(content), want)
	}
}
