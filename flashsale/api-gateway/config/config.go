// Package config loads and validates the API gateway's configuration.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/flashsale/common/env"
)

// minJWTSecretLength is the shortest secret accepted for signing tokens. A
// short or empty secret makes tokens trivially forgeable, so the gateway
// refuses to start with one.
const minJWTSecretLength = 16

// Config holds the API gateway's configuration.
type Config struct {
	Port               string
	JWTSecret          string
	TokenTTL           time.Duration
	PurchaseServiceURL string
	// InternalToken is presented to the purchase service, which trusts this
	// gateway to have authenticated the buyer.
	InternalToken string
	PostgresURL   string
	RedisAddr     string
	RedisPassword string
	// CredentialCacheTTL is how long a credential lookup stays cached.
	CredentialCacheTTL time.Duration
	// RateLimitRequests is the number of requests allowed per client address
	// within RateLimitWindow.
	RateLimitRequests int
	RateLimitWindow   time.Duration
	ShutdownTimeout   time.Duration
}

// Load reads configuration from the environment and validates it. Secrets and
// connection strings have no defaults: a gateway that starts with an empty JWT
// secret would accept forged tokens, so it must fail loudly instead.
func Load() (*Config, error) {
	var errs []error

	jwtSecret, err := env.Require("JWT_SECRET")
	errs = append(errs, err)

	purchaseURL, err := env.Require("PURCHASE_SERVICE_URL")
	errs = append(errs, err)

	internalToken, err := env.Require("INTERNAL_API_TOKEN")
	errs = append(errs, err)

	postgresURL, err := env.Require("POSTGRES_URL")
	errs = append(errs, err)

	redisAddr, err := env.Require("REDIS_ADDR")
	errs = append(errs, err)

	tokenTTL, err := env.Duration("TOKEN_TTL", 24*time.Hour)
	errs = append(errs, err)

	cacheTTL, err := env.Duration("CREDENTIAL_CACHE_TTL", time.Hour)
	errs = append(errs, err)

	rateLimitRequests, err := env.Int("RATE_LIMIT_REQUESTS", 120)
	errs = append(errs, err)

	rateLimitWindow, err := env.Duration("RATE_LIMIT_WINDOW", time.Minute)
	errs = append(errs, err)

	shutdownTimeout, err := env.Duration("SHUTDOWN_TIMEOUT", 15*time.Second)
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:               env.Get("API_GATEWAY_PORT", "8080"),
		JWTSecret:          jwtSecret,
		TokenTTL:           tokenTTL,
		PurchaseServiceURL: purchaseURL,
		InternalToken:      internalToken,
		PostgresURL:        postgresURL,
		RedisAddr:          redisAddr,
		RedisPassword:      env.Get("REDIS_PASSWORD", ""),
		CredentialCacheTTL: cacheTTL,
		RateLimitRequests:  rateLimitRequests,
		RateLimitWindow:    rateLimitWindow,
		ShutdownTimeout:    shutdownTimeout,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks values that must hold regardless of where they came from.
func (c *Config) Validate() error {
	if len(c.JWTSecret) < minJWTSecretLength {
		return fmt.Errorf("JWT_SECRET must be at least %d characters", minJWTSecretLength)
	}
	if c.RateLimitRequests <= 0 {
		return fmt.Errorf("RATE_LIMIT_REQUESTS must be positive, got %d", c.RateLimitRequests)
	}
	if c.RateLimitWindow <= 0 {
		return fmt.Errorf("RATE_LIMIT_WINDOW must be positive, got %s", c.RateLimitWindow)
	}
	if c.TokenTTL <= 0 {
		return fmt.Errorf("TOKEN_TTL must be positive, got %s", c.TokenTTL)
	}
	return nil
}
