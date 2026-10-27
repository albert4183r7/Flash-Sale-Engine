// Command api-gateway is the flash sale's public entry point. It authenticates
// buyers, rate limits traffic, forwards purchases to the purchase service and
// serves order lookups and cancellations from PostgreSQL.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/router"
	"github.com/flashsale/common/postgres"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("api gateway: %v", err)
	}
	log.Println("api gateway stopped")
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

	log.Println("starting api gateway")

	db, err := postgres.Open(ctx, cfg.PostgresURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	cache := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	})
	defer func() { _ = cache.Close() }()

	// Redis only accelerates credential lookups here, so the gateway still
	// serves traffic without it, falling back to PostgreSQL on every login.
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	err = cache.Ping(pingCtx).Err()
	cancelPing()
	if err != nil {
		log.Printf("warning: Redis unavailable, logins will read from PostgreSQL: %v", err)
	} else {
		log.Println("connected to Redis")
	}

	engine, limiter := router.Setup(cfg, db, cache)
	defer limiter.Stop()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("api gateway listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	log.Println("shutting down api gateway")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
