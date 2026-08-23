// Package config loads and validates the order worker's configuration.
package config

import (
	"errors"
	"time"

	"github.com/flashsale/common/env"
)

// Config holds the order worker's configuration.
type Config struct {
	RabbitMQURL string
	PostgresURL string
	// DBTimeout bounds each database operation performed while processing a
	// message, so a stalled database cannot block the consumer forever.
	DBTimeout time.Duration
	// ShutdownTimeout bounds how long the worker waits for an in-flight message
	// to finish after a termination signal.
	ShutdownTimeout time.Duration
}

// Load reads configuration from the environment and validates it. It returns an
// error rather than falling back to defaults for connection strings, so that a
// misconfigured worker fails at startup instead of at the first message.
func Load() (*Config, error) {
	var errs []error

	rabbitURL, err := env.Require("RABBITMQ_URL")
	errs = append(errs, err)

	postgresURL, err := env.Require("POSTGRES_URL")
	errs = append(errs, err)

	dbTimeout, err := env.Duration("DB_TIMEOUT", 10*time.Second)
	errs = append(errs, err)

	shutdownTimeout, err := env.Duration("SHUTDOWN_TIMEOUT", 15*time.Second)
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return &Config{
		RabbitMQURL:     rabbitURL,
		PostgresURL:     postgresURL,
		DBTimeout:       dbTimeout,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}
