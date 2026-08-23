package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/flashsale/api-gateway/config"
)

// setValidEnv exports a complete, valid configuration.
func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "a-test-secret-at-least-16-chars")
	t.Setenv("INTERNAL_API_TOKEN", "internal-token")
	t.Setenv("PURCHASE_SERVICE_URL", "http://localhost:8081")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("REDIS_ADDR", "localhost:6379")
}

func TestLoadAppliesDefaults(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.TokenTTL != 24*time.Hour {
		t.Errorf("TokenTTL = %s, want 24h0m0s", cfg.TokenTTL)
	}
	if cfg.RateLimitRequests != 120 {
		t.Errorf("RateLimitRequests = %d, want 120", cfg.RateLimitRequests)
	}
	if cfg.RateLimitWindow != time.Minute {
		t.Errorf("RateLimitWindow = %s, want 1m0s", cfg.RateLimitWindow)
	}
}

func TestLoadReadsOverrides(t *testing.T) {
	setValidEnv(t)
	t.Setenv("API_GATEWAY_PORT", "9090")
	t.Setenv("TOKEN_TTL", "15m")
	t.Setenv("RATE_LIMIT_REQUESTS", "10")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.TokenTTL != 15*time.Minute {
		t.Errorf("TokenTTL = %s, want 15m0s", cfg.TokenTTL)
	}
	if cfg.RateLimitRequests != 10 {
		t.Errorf("RateLimitRequests = %d, want 10", cfg.RateLimitRequests)
	}
}

// Every secret and connection string must be required. Starting with an empty
// JWT secret would mean accepting tokens anybody can forge, so the gateway has
// to refuse rather than fall back to a default.
func TestLoadRequiresSecrets(t *testing.T) {
	// POSTGRES_URL is deliberately absent: the DSN is assembled from
	// POSTGRES_* parts when no complete URL is supplied.
	required := []string{
		"JWT_SECRET",
		"INTERNAL_API_TOKEN",
		"PURCHASE_SERVICE_URL",
		"REDIS_ADDR",
	}

	for _, key := range required {
		t.Run(key, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv(key, "")

			_, err := config.Load()
			if err == nil {
				t.Fatalf("Load() without %s = nil, want an error", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error %q does not mention %s", err, key)
			}
		})
	}
}

// Load must report every missing variable at once, so a misconfigured deploy is
// fixed in one pass instead of one variable per restart.
func TestLoadReportsAllMissingVariables(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("INTERNAL_API_TOKEN", "")
	t.Setenv("PURCHASE_SERVICE_URL", "")
	t.Setenv("REDIS_ADDR", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() = nil, want an error")
	}
	for _, key := range []string{"JWT_SECRET", "INTERNAL_API_TOKEN", "PURCHASE_SERVICE_URL", "REDIS_ADDR"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}

// A short secret is nearly as bad as none at all.
func TestLoadRejectsShortJWTSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_SECRET", "short")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() with a short JWT_SECRET = nil, want an error")
	}
}

func TestLoadRejectsUnparseableValues(t *testing.T) {
	tests := []struct{ key, value string }{
		{"TOKEN_TTL", "forever"},
		{"RATE_LIMIT_REQUESTS", "many"},
		{"RATE_LIMIT_WINDOW", "1 minute"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv(tt.key, tt.value)

			if _, err := config.Load(); err == nil {
				t.Fatalf("Load() with %s=%q = nil, want an error", tt.key, tt.value)
			}
		})
	}
}

func TestValidateRejectsNonPositiveLimits(t *testing.T) {
	base := config.Config{
		JWTSecret:         "a-test-secret-at-least-16-chars",
		TokenTTL:          time.Hour,
		RateLimitRequests: 10,
		RateLimitWindow:   time.Minute,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() on a valid config = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"zero rate limit", func(c *config.Config) { c.RateLimitRequests = 0 }},
		{"negative rate limit", func(c *config.Config) { c.RateLimitRequests = -1 }},
		{"zero window", func(c *config.Config) { c.RateLimitWindow = 0 }},
		{"zero token ttl", func(c *config.Config) { c.TokenTTL = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
		})
	}
}

// The database URL may be supplied whole or as parts. Both must reach the same
// place, so that Docker Compose can redirect a service by host alone.
func TestLoadResolvesPostgresFromPartsOrURL(t *testing.T) {
	t.Run("from parts", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("POSTGRES_URL", "")
		t.Setenv("POSTGRES_HOST", "localhost")
		t.Setenv("POSTGRES_PASSWORD", "pw")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		want := "postgres://postgres:pw@localhost:5432/flashsale?sslmode=disable"
		if cfg.PostgresURL != want {
			t.Errorf("PostgresURL = %q, want %q", cfg.PostgresURL, want)
		}
	})

	t.Run("explicit url wins", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("POSTGRES_URL", "postgres://app@managed.example.com:5432/sales?sslmode=require")
		t.Setenv("POSTGRES_HOST", "ignored")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.PostgresURL != "postgres://app@managed.example.com:5432/sales?sslmode=require" {
			t.Errorf("PostgresURL = %q, want the explicit POSTGRES_URL", cfg.PostgresURL)
		}
	})
}
