// Package config loads FaultHub configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

type Config struct {
	Addr              string
	DataDir           string
	AdminPassword     string
	AdminPasswordHash string
	IngestRatePerMin  int
	IngestBurst       int
	LoginRatePerMin   int
	MaxBodyBytes      int64
	TrustProxy        bool
	CookieSecure      bool
	SessionTTL        time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:             ":8080",
		DataDir:          "/data",
		IngestRatePerMin: 30,
		IngestBurst:      10,
		LoginRatePerMin:  5,
		MaxBodyBytes:     1 << 20,
		TrustProxy:       true,
		CookieSecure:     true,
		SessionTTL:       168 * time.Hour,
	}
	var err error
	if c.Addr, err = str(getenv, "FAULTHUB_ADDR", c.Addr); err != nil {
		return c, err
	}
	if c.DataDir, err = str(getenv, "FAULTHUB_DATA_DIR", c.DataDir); err != nil {
		return c, err
	}
	c.AdminPassword = getenv("FAULTHUB_ADMIN_PASSWORD")
	c.AdminPasswordHash = getenv("FAULTHUB_ADMIN_PASSWORD_HASH")
	if c.AdminPassword == "" && c.AdminPasswordHash == "" {
		return c, errors.New("set FAULTHUB_ADMIN_PASSWORD or FAULTHUB_ADMIN_PASSWORD_HASH")
	}
	if c.AdminPassword != "" && c.AdminPasswordHash != "" {
		return c, errors.New("set exactly one of FAULTHUB_ADMIN_PASSWORD / FAULTHUB_ADMIN_PASSWORD_HASH, not both")
	}
	if c.IngestRatePerMin, err = integer(getenv, "FAULTHUB_INGEST_RATE_PER_MIN", c.IngestRatePerMin); err != nil {
		return c, err
	}
	if c.IngestBurst, err = integer(getenv, "FAULTHUB_INGEST_BURST", c.IngestBurst); err != nil {
		return c, err
	}
	if c.LoginRatePerMin, err = integer(getenv, "FAULTHUB_LOGIN_RATE_PER_MIN", c.LoginRatePerMin); err != nil {
		return c, err
	}
	if c.MaxBodyBytes, err = integer64(getenv, "FAULTHUB_MAX_BODY_BYTES", c.MaxBodyBytes); err != nil {
		return c, err
	}
	if c.TrustProxy, err = boolean(getenv, "FAULTHUB_TRUST_PROXY", c.TrustProxy); err != nil {
		return c, err
	}
	if c.CookieSecure, err = boolean(getenv, "FAULTHUB_COOKIE_SECURE", c.CookieSecure); err != nil {
		return c, err
	}
	if c.SessionTTL, err = duration(getenv, "FAULTHUB_SESSION_TTL", c.SessionTTL); err != nil {
		return c, err
	}
	if c.IngestRatePerMin < 1 || c.IngestBurst < 1 || c.LoginRatePerMin < 1 {
		return c, fmt.Errorf("rate limits must be >= 1")
	}
	if c.MaxBodyBytes < 1024 {
		return c, fmt.Errorf("FAULTHUB_MAX_BODY_BYTES must be >= 1024")
	}
	if c.SessionTTL <= 0 {
		return c, fmt.Errorf("FAULTHUB_SESSION_TTL must be positive")
	}
	return c, nil
}

func str(getenv func(string) string, key, def string) (string, error) {
	if v := getenv(key); v != "" {
		return v, nil
	}
	return def, nil
}

func integer(getenv func(string) string, key string, def int) (int, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func integer64(getenv func(string) string, key string, def int64) (int64, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func boolean(getenv func(string) string, key string, def bool) (bool, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}

func duration(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}
