package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	encoded, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=") {
		t.Fatalf("bad format: %q", encoded)
	}
	if !VerifyPassword(encoded, "hunter2") {
		t.Fatal("correct password must verify")
	}
	if VerifyPassword(encoded, "hunter3") {
		t.Fatal("wrong password must fail")
	}
	if VerifyPassword(encoded, "") {
		t.Fatal("empty password must fail")
	}
}

func TestUniqueSalts(t *testing.T) {
	a, _ := HashPassword("pw")
	b, _ := HashPassword("pw")
	if a == b {
		t.Fatal("salts must differ")
	}
	if !VerifyPassword(a, "pw") || !VerifyPassword(b, "pw") {
		t.Fatal("both must verify")
	}
}

func TestVerifyMalformed(t *testing.T) {
	for _, bad := range []string{
		"", "plaintext", "$argon2i$v=19$m=65536,t=3,p=4$abc$def",
		"$argon2id$v=19$m=65536,t=3,p=4$", "$argon2id$v=19$m=65536,t=3,p=4$!!!$???",
		"$argon2id$v=19$m=0,t=3,p=4$YWJj$YWJj",
	} {
		if VerifyPassword(bad, "pw") {
			t.Fatalf("malformed %q must not verify", bad)
		}
	}
}
