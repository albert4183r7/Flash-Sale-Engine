// Package config loads and validates the purchase service's configuration.
package config

import (
	"errors"
	"time"

	"github.com/flashsale/common/env"
)

// Config holds the purchase service's configuration.
type Config struct {
	Port          string
	RedisAddr     string
	RedisPassword string
	RabbitMQURL   string
	// PostgresURL is used only to warm the Redis stock counters at startup.
	PostgresURL string
	// InternalToken guards the routes the API gateway calls on this service.
	// The purchase service trusts the user ID in its request bodies, so those
	// routes must never be reachable by an untrusted caller.
	InternalToken string
	// IdempotencyTTL is how long a buyer's claim on a product is held, which
	// bounds how quickly the same order can be submitted twice.
	IdempotencyTTL  time.Duration
	ShutdownTimeout time.Duration
}

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	var errs []error

	redisAddr, err := env.Require("REDIS_ADDR")
	errs = append(errs, err)

	rabbitURL, err := env.Require("RABBITMQ_URL")
	errs = append(errs, err)

	postgresURL, err := env.Require("POSTGRES_URL")
	errs = append(errs, err)

	internalToken, err := env.Require("INTERNAL_API_TOKEN")
	errs = append(errs, err)

	idempotencyTTL, err := env.Duration("IDEMPOTENCY_TTL", 2*time.Minute)
	errs = append(errs, err)

	shutdownTimeout, err := env.Duration("SHUTDOWN_TIMEOUT", 15*time.Second)
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return &Config{
		Port:            env.Get("PURCHASE_SERVICE_PORT", "8081"),
		RedisAddr:       redisAddr,
		RedisPassword:   env.Get("REDIS_PASSWORD", ""),
		RabbitMQURL:     rabbitURL,
		PostgresURL:     postgresURL,
		InternalToken:   internalToken,
		IdempotencyTTL:  idempotencyTTL,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}
