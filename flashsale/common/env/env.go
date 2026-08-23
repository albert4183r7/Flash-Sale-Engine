// Package env provides small helpers for reading service configuration from
// environment variables. Each service composes its own typed config from these
// helpers rather than sharing a single configuration struct, because the
// services have genuinely different configuration needs.
package env

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Get returns the value of key, or fallback when the variable is unset or empty.
func Get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// MissingError reports a required environment variable that was not set.
type MissingError struct {
	Key string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf("required environment variable %s is not set", e.Key)
}

// Require returns the value of key, or a *MissingError when it is unset or empty.
// Secrets and connection strings have no safe default, so they must be required
// rather than silently defaulted.
func Require(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", &MissingError{Key: key}
	}
	return v, nil
}

// Int returns the value of key parsed as an int, or fallback when the variable
// is unset. A value that is set but unparseable is reported as an error so that
// a typo in configuration fails loudly instead of silently using the default.
func Int(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s: %q is not a valid integer: %w", key, raw, err)
	}
	return v, nil
}

// Duration returns the value of key parsed as a time.Duration (for example
// "30s" or "1h"), or fallback when the variable is unset.
func Duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s: %q is not a valid duration: %w", key, raw, err)
	}
	return v, nil
}
