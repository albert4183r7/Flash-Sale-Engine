// Package postgres manages the order worker's PostgreSQL connection.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // database/sql driver
)

const (
	connectAttempts = 5
	connectBackoff  = 2 * time.Second
)

// Connect opens a PostgreSQL connection pool and waits for the server to become
// reachable, retrying to tolerate the database still starting up alongside the
// worker. The pool is opened once and only the reachability check is retried.
func Connect(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)

	for attempt := 1; attempt <= connectAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.PingContext(pingCtx)
		cancel()
		if err == nil {
			log.Println("connected to PostgreSQL")
			return db, nil
		}
		log.Printf("postgres not ready (attempt %d/%d): %v", attempt, connectAttempts, err)

		select {
		case <-ctx.Done():
			_ = db.Close()
			return nil, ctx.Err()
		case <-time.After(connectBackoff):
		}
	}

	_ = db.Close()
	return nil, fmt.Errorf("connect to postgres after %d attempts: %w", connectAttempts, err)
}
