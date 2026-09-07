package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"faulthub/internal/auth"
)

const (
	envHashKey = "FAULTHUB_ADMIN_PASSWORD_HASH"
	envPwKey   = "FAULTHUB_ADMIN_PASSWORD"
)

func runSetPassword(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: faulthub set-password [env-file]")
	}
	path := ".env"
	if len(args) == 1 {
		path = args[0]
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Password: ")
	pw, err := readLine(reader)
	if err != nil {
		return err
	}
	fmt.Print("Confirm: ")
	confirm, err := readLine(reader)
	if err != nil {
		return err
	}
	if pw == "" {
		return errors.New("password must not be empty")
	}
	if pw != confirm {
		return errors.New("passwords do not match")
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	return writeEnvHash(path, h)
}

func writeEnvHash(path, hash string) error {
	escaped := composeEscape(hash)
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(envHashKey+"="+escaped+"\n"), 0o600)
	}
	if err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(rewriteEnv(string(content), escaped)), fi.Mode().Perm()); err != nil {
		return err
	}
	fmt.Printf("updated %s in %s\n", envHashKey, path)
	return nil
}

func composeEscape(v string) string {
	return strings.ReplaceAll(v, "$", "$$")
}

func rewriteEnv(content, escapedHash string) string {
	hashLine := envHashKey + "=" + escapedHash
	var out []string
	replaced := false
	for _, ln := range strings.Split(content, "\n") {
		switch envKey(ln) {
		case envPwKey:
			continue
		case envHashKey:
			if !replaced {
				out = append(out, hashLine)
				replaced = true
			}
			continue
		}
		out = append(out, ln)
	}
	if !replaced {
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		out = append(out, hashLine)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	res := strings.Join(out, "\n")
	if strings.HasSuffix(content, "\n") {
		res += "\n"
	}
	return res
}

func envKey(ln string) string {
	trimmed := strings.TrimSpace(ln)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	i := strings.Index(trimmed, "=")
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(trimmed[:i])
}
