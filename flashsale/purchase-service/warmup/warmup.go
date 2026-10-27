// Package warmup seeds the Redis stock counters from the durable product stock
// in PostgreSQL, so the purchase service can serve a flash sale entirely from
// memory once it has started.
package warmup

import (
	"context"
	"fmt"
	"log"

	"github.com/flashsale/common/postgres"
	"github.com/google/uuid"
)

// StockInitializer writes a product's starting stock.
type StockInitializer interface {
	InitializeStock(ctx context.Context, productID uuid.UUID, stock int) error
}

// LoadStock copies every product's stock from PostgreSQL into the stock store.
// The database connection is opened and closed here because it is needed only
// at startup: the purchase service serves requests from Redis alone.
func LoadStock(ctx context.Context, postgresURL string, stock StockInitializer) error {
	db, err := postgres.Open(ctx, postgresURL)
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
