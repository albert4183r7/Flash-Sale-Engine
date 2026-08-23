// Command order-worker consumes order events from RabbitMQ and persists them to
// PostgreSQL, deducting durable product stock as part of the same transaction.
package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"

	"github.com/flashsale/order-worker/config"
	"github.com/flashsale/order-worker/consumer"
	"github.com/flashsale/order-worker/postgres"
	"github.com/flashsale/order-worker/repository"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("order worker: %v", err)
	}
	log.Println("order worker stopped")
}

func run() error {
	// A missing .env is normal in containers, where configuration comes from the
	// environment directly.
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("no .env file found, using environment variables")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Println("starting order worker")

	db, err := postgres.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	orders := repository.NewOrderRepository(db)

	c, err := consumer.New(cfg.RabbitMQURL, orders, cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer c.Close()

	// Run blocks until shutdown is requested or the broker connection drops.
	// Losing the connection is reported as an error so the process exits and its
	// supervisor restarts it, rather than idling while consuming nothing.
	if err := c.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
