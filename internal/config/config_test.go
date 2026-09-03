package config

import (
	"strings"
	"testing"
	"time"
)

func env(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Load(env("FAULTHUB_ADMIN_PASSWORD", "hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.DataDir != "/data" || c.AdminPassword != "hunter2" {
		t.Fatalf("got %+v", c)
	}
	if c.IngestRatePerMin != 30 || c.IngestBurst != 10 || c.LoginRatePerMin != 5 {
		t.Fatalf("rate defaults wrong: %+v", c)
	}
	if c.MaxBodyBytes != 1<<20 {
		t.Fatalf("MaxBodyBytes = %d", c.MaxBodyBytes)
	}
	if !c.TrustProxy || !c.CookieSecure {
		t.Fatalf("bool defaults wrong: %+v", c)
	}
	if c.SessionTTL != 168*time.Hour {
		t.Fatalf("SessionTTL = %v", c.SessionTTL)
	}
}

func TestOverrides(t *testing.T) {
	c, err := Load(env(
		"FAULTHUB_ADMIN_PASSWORD_HASH", "$argon2id$x",
		"FAULTHUB_ADDR", ":9000",
		"FAULTHUB_DATA_DIR", "/var/lib/fh",
		"FAULTHUB_INGEST_RATE_PER_MIN", "60",
		"FAULTHUB_INGEST_BURST", "20",
		"FAULTHUB_LOGIN_RATE_PER_MIN", "10",
		"FAULTHUB_MAX_BODY_BYTES", "5242880",
		"FAULTHUB_TRUST_PROXY", "false",
		"FAULTHUB_COOKIE_SECURE", "false",
		"FAULTHUB_SESSION_TTL", "24h",
	))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":9000" || c.DataDir != "/var/lib/fh" || c.AdminPasswordHash != "$argon2id$x" || c.AdminPassword != "" {
		t.Fatalf("got %+v", c)
	}
	if c.IngestRatePerMin != 60 || c.IngestBurst != 20 || c.LoginRatePerMin != 10 || c.MaxBodyBytes != 5242880 {
		t.Fatalf("got %+v", c)
	}
	if c.TrustProxy || c.CookieSecure || c.SessionTTL != 24*time.Hour {
		t.Fatalf("got %+v", c)
	}
}

func TestPasswordRequired(t *testing.T) {
	_, err := Load(env())
	if err == nil || !strings.Contains(err.Error(), "FAULTHUB_ADMIN_PASSWORD") {
		t.Fatalf("want password-required error, got %v", err)
	}
}

func TestBothPasswordSourcesRejected(t *testing.T) {
	_, err := Load(env("FAULTHUB_ADMIN_PASSWORD", "a", "FAULTHUB_ADMIN_PASSWORD_HASH", "b"))
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("want both-set error, got %v", err)
	}
}

func TestInvalidValues(t *testing.T) {
	cases := [][]string{
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_INGEST_RATE_PER_MIN", "abc"},
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_INGEST_RATE_PER_MIN", "0"},
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_INGEST_BURST", "0"},
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_LOGIN_RATE_PER_MIN", "-1"},
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_MAX_BODY_BYTES", "100"},
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_SESSION_TTL", "nope"},
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_SESSION_TTL", "-1h"},
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_TRUST_PROXY", "maybe"},
		{"FAULTHUB_ADMIN_PASSWORD", "x", "FAULTHUB_COOKIE_SECURE", "yes"},
	}
	for _, kv := range cases {
		if _, err := Load(env(kv...)); err == nil {
			t.Fatalf("expected error for %v", kv)
		}
	}
}
