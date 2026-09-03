// Package auth hashes and verifies the admin password with argon2id.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memKiB  = 19456
	iter    = 2
	threads = 1
	saltLen = 16
	keyLen  = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, iter, memKiB, threads, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memKiB, iter, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return false
	}
	var mem, iters, par int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iters, &par); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	if mem <= 0 || iters <= 0 || par <= 0 || len(salt) == 0 || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iters), uint32(mem), uint8(par), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
