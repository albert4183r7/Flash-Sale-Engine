package env_test

import (
	"errors"
	"testing"
	"time"

	"github.com/flashsale/common/env"
)

func TestGet(t *testing.T) {
	t.Setenv("FLASHSALE_TEST_VALUE", "set")
	if got := env.Get("FLASHSALE_TEST_VALUE", "fallback"); got != "set" {
		t.Errorf("Get() = %q, want %q", got, "set")
	}
	if got := env.Get("FLASHSALE_TEST_ABSENT", "fallback"); got != "fallback" {
		t.Errorf("Get() = %q, want %q", got, "fallback")
	}
}

// An empty variable must be treated as unset: an exported but blank secret is
// the failure mode this guards against.
func TestGetTreatsEmptyAsUnset(t *testing.T) {
	t.Setenv("FLASHSALE_TEST_EMPTY", "")
	if got := env.Get("FLASHSALE_TEST_EMPTY", "fallback"); got != "fallback" {
		t.Errorf("Get() = %q, want %q", got, "fallback")
	}
}

func TestRequire(t *testing.T) {
	t.Setenv("FLASHSALE_TEST_SECRET", "s3cret")
	got, err := env.Require("FLASHSALE_TEST_SECRET")
	if err != nil {
		t.Fatalf("Require() error = %v", err)
	}
	if got != "s3cret" {
		t.Errorf("Require() = %q, want %q", got, "s3cret")
	}

	if _, err := env.Require("FLASHSALE_TEST_MISSING"); err == nil {
		t.Fatal("Require() on a missing variable = nil, want an error")
	} else {
		var missing *env.MissingError
		if !errors.As(err, &missing) {
			t.Fatalf("Require() error = %T, want *env.MissingError", err)
		}
		if missing.Key != "FLASHSALE_TEST_MISSING" {
			t.Errorf("MissingError.Key = %q, want %q", missing.Key, "FLASHSALE_TEST_MISSING")
		}
	}
}

func TestInt(t *testing.T) {
	t.Setenv("FLASHSALE_TEST_INT", "42")
	got, err := env.Int("FLASHSALE_TEST_INT", 7)
	if err != nil || got != 42 {
		t.Fatalf("Int() = (%d, %v), want (42, nil)", got, err)
	}

	got, err = env.Int("FLASHSALE_TEST_INT_ABSENT", 7)
	if err != nil || got != 7 {
		t.Fatalf("Int() = (%d, %v), want (7, nil)", got, err)
	}

	// A typo must fail loudly rather than silently using the default.
	t.Setenv("FLASHSALE_TEST_INT_BAD", "12O")
	if _, err := env.Int("FLASHSALE_TEST_INT_BAD", 7); err == nil {
		t.Fatal("Int() on an unparseable value = nil, want an error")
	}
}

func TestDuration(t *testing.T) {
	t.Setenv("FLASHSALE_TEST_DURATION", "90s")
	got, err := env.Duration("FLASHSALE_TEST_DURATION", time.Minute)
	if err != nil || got != 90*time.Second {
		t.Fatalf("Duration() = (%s, %v), want (1m30s, nil)", got, err)
	}

	got, err = env.Duration("FLASHSALE_TEST_DURATION_ABSENT", time.Minute)
	if err != nil || got != time.Minute {
		t.Fatalf("Duration() = (%s, %v), want (1m0s, nil)", got, err)
	}

	t.Setenv("FLASHSALE_TEST_DURATION_BAD", "5 minutes")
	if _, err := env.Duration("FLASHSALE_TEST_DURATION_BAD", time.Minute); err == nil {
		t.Fatal("Duration() on an unparseable value = nil, want an error")
	}
}
