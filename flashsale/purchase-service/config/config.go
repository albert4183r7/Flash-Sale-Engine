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
	// IdempotencyTTL is how long a buyer's claim on a product is held. Within
	// it, resubmitting the purchase returns the original order.
	IdempotencyTTL time.Duration
	// PendingOrderTTL is how long an accepted order stays readable from Redis
	// while it waits for the worker to store it.
	PendingOrderTTL time.Duration
	// OutboxRelayInterval is how often the outbox is checked for events whose
	// publish was interrupted.
	OutboxRelayInterval time.Duration
	// OutboxRetryAfter is how old an outbox event must be before the relay
	// publishes it again. It must comfortably exceed the publisher's confirm
	// timeout, so a request still waiting on the broker is not raced.
	OutboxRetryAfter time.Duration
	ShutdownTimeout  time.Duration
}

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	var errs []error

	redisAddr, err := env.Require("REDIS_ADDR")
	errs = append(errs, err)

	// Assembled from RABBITMQ_* parts unless RABBITMQ_URL overrides them.
	rabbitURL, err := env.AMQPURL()
	errs = append(errs, err)

	// Assembled from POSTGRES_* parts unless POSTGRES_URL overrides them.
	postgresURL, err := env.PostgresDSN()
	errs = append(errs, err)

	internalToken, err := env.Require("INTERNAL_API_TOKEN")
	errs = append(errs, err)

	idempotencyTTL, err := env.Duration("IDEMPOTENCY_TTL", 2*time.Minute)
	errs = append(errs, err)

	pendingOrderTTL, err := env.Duration("PENDING_ORDER_TTL", 15*time.Minute)
	errs = append(errs, err)

	relayInterval, err := env.Duration("OUTBOX_RELAY_INTERVAL", 10*time.Second)
	errs = append(errs, err)

	retryAfter, err := env.Duration("OUTBOX_RETRY_AFTER", 30*time.Second)
	errs = append(errs, err)

	shutdownTimeout, err := env.Duration("SHUTDOWN_TIMEOUT", 15*time.Second)
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return &Config{
		Port:                env.Get("PURCHASE_SERVICE_PORT", "8081"),
		RedisAddr:           redisAddr,
		RedisPassword:       env.Get("REDIS_PASSWORD", ""),
		RabbitMQURL:         rabbitURL,
		PostgresURL:         postgresURL,
		InternalToken:       internalToken,
		IdempotencyTTL:      idempotencyTTL,
		PendingOrderTTL:     pendingOrderTTL,
		OutboxRelayInterval: relayInterval,
		OutboxRetryAfter:    retryAfter,
		ShutdownTimeout:     shutdownTimeout,
	}, nil
}
