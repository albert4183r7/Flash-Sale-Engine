// Command api-gateway is the flash sale's public entry point. It authenticates
// buyers, rate limits traffic, forwards purchases to the purchase service and
// serves order lookups and cancellations from PostgreSQL.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/router"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq" // database/sql driver
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

	db, err := openDatabase(ctx, cfg.PostgresURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	log.Println("connected to PostgreSQL")

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

// openDatabase opens the connection pool and verifies the database is
// reachable before the gateway starts accepting traffic.
func openDatabase(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	// Bounding the pool keeps a traffic spike from exhausting the database's
	// connection slots.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}
