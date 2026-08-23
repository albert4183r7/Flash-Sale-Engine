// Package warmup seeds the Redis stock counters from the durable product stock
// in PostgreSQL, so the purchase service can serve a flash sale entirely from
// memory once it has started.
package warmup

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq" // database/sql driver
)

const (
	connectAttempts = 5
	connectBackoff  = 2 * time.Second
)

// StockInitializer writes a product's starting stock.
type StockInitializer interface {
	InitializeStock(ctx context.Context, productID uuid.UUID, stock int) error
}

// LoadStock copies every product's stock from PostgreSQL into the stock store.
// The database connection is opened and closed here because it is needed only
// at startup: the purchase service serves requests from Redis alone.
func LoadStock(ctx context.Context, postgresURL string, stock StockInitializer) error {
	db, err := connect(ctx, postgresURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(ctx, `SELECT id, stock FROM products`)
	if err != nil {
		return fmt.Errorf("query products: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var loaded int
	for rows.Next() {
		var (
			id    uuid.UUID
			units int
		)
		if err := rows.Scan(&id, &units); err != nil {
			return fmt.Errorf("scan product row: %w", err)
		}
		if err := stock.InitializeStock(ctx, id, units); err != nil {
			return err
		}
		loaded++
	}
	// rows.Err reports an error that ended iteration early, which would
	// otherwise look like a successful but incomplete warm-up.
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate products: %w", err)
	}

	log.Printf("warmed up stock counters for %d product(s)", loaded)
	return nil
}

func connect(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	for attempt := 1; attempt <= connectAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.PingContext(pingCtx)
		cancel()
		if err == nil {
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
