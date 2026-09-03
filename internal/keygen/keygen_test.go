package keygen

import (
	"strings"
	"testing"
)

func TestNewAPIKey(t *testing.T) {
	key, hash, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "fh_") || len(key) != 3+43 {
		t.Fatalf("bad key format: %q", key)
	}
	if got := HashToken(key); got != hash {
		t.Fatalf("hash mismatch: %q vs %q", got, hash)
	}
	k2, _, _ := NewAPIKey()
	if k2 == key {
		t.Fatal("keys must be unique")
	}
}

func TestHashTokenStable(t *testing.T) {
	a, b := HashToken("x"), HashToken("x")
	if a != b || len(a) != 64 {
		t.Fatalf("hash must be stable 64-hex: %q %q", a, b)
	}
}

func TestNewToken(t *testing.T) {
	token, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 {
		t.Fatalf("token len = %d", len(token))
	}
	if HashToken(token) != hash {
		t.Fatal("hash mismatch")
	}
}
