// Command purchase-service serves the hot path of the flash sale: it reserves
// stock in Redis and queues accepted orders on RabbitMQ for durable processing.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/purchase-service/config"
	"github.com/flashsale/purchase-service/publisher"
	"github.com/flashsale/purchase-service/redis"
	"github.com/flashsale/purchase-service/router"
	"github.com/flashsale/purchase-service/service"
	"github.com/flashsale/purchase-service/warmup"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("purchase service: %v", err)
	}
	log.Println("purchase service stopped")
}

func run() error {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("no .env file found, using environment variables")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Println("starting purchase service")

	stockStore := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword)
	defer func() { _ = stockStore.Close() }()

	// Redis holds the authoritative stock during a sale, so there is nothing to
	// serve without it.
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	err = stockStore.Ping(pingCtx)
	cancelPing()
	if err != nil {
		return err
	}
	log.Println("connected to Redis")

	// Without RabbitMQ an accepted purchase could never become an order, so the
	// service refuses to start rather than reserving stock it cannot fulfil.
	events, err := publisher.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer events.Close()

	warmCtx, cancelWarm := context.WithTimeout(ctx, 30*time.Second)
	err = warmup.LoadStock(warmCtx, cfg.PostgresURL, stockStore)
	cancelWarm()
	if err != nil {
		return err
	}

	purchases := service.NewPurchase(stockStore, events, cfg.IdempotencyTTL)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router.Setup(cfg, stockStore, purchases),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("purchase service listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Surface a failure to bind the port instead of leaving a process that is
	// running but serving nothing.
	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	log.Println("shutting down purchase service")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
