// Package postgres opens the PostgreSQL connection pool used by every service,
// so pool limits and startup retries are defined once.
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
	pingTimeout     = 5 * time.Second
)

// Open opens a bounded connection pool and waits for the server to answer,
// retrying to tolerate a database that is still starting alongside the
// service. The pool is opened once and only the reachability check is retried.
func Open(ctx context.Context, url string) (*sql.DB, error) {
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

	for attempt := 1; attempt <= connectAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
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
