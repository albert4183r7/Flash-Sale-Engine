// Package warmup seeds the Redis stock counters from the durable product stock
// in PostgreSQL, so the purchase service can serve a flash sale entirely from
// memory.
package warmup

import (
	"context"
	"fmt"
	"log"

	"github.com/flashsale/common/postgres"
	"github.com/google/uuid"
)

// StockInitializer writes a product's starting stock unless a counter for it
// already exists, reporting whether it created one.
type StockInitializer interface {
	InitializeStock(ctx context.Context, productID uuid.UUID, stock int) (bool, error)
}

// Result counts what a load did.
type Result struct {
	// Loaded is the number of products whose counter was created.
	Loaded int
	// Kept is the number of products whose live counter was left untouched.
	Kept int
}

// LoadStock creates a stock counter for every product that does not have one
// yet. Existing counters are kept, because they already account for orders
// still on their way to PostgreSQL; this makes the load safe to run on every
// restart, and again whenever new products are added.
//
// The database connection is opened and closed here because it is needed only
// for the load: the purchase service serves requests from Redis alone.
func LoadStock(ctx context.Context, postgresURL string, stock StockInitializer) (Result, error) {
	var result Result

	db, err := postgres.Open(ctx, postgresURL)
	if err != nil {
		return result, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(ctx, `SELECT id, stock FROM products`)
	if err != nil {
		return result, fmt.Errorf("query products: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			id    uuid.UUID
			units int
		)
		if err := rows.Scan(&id, &units); err != nil {
			return result, fmt.Errorf("scan product row: %w", err)
		}
		created, err := stock.InitializeStock(ctx, id, units)
		if err != nil {
			return result, err
		}
		if created {
			result.Loaded++
		} else {
			result.Kept++
		}
	}
	// rows.Err reports an error that ended iteration early, which would
	// otherwise look like a successful but incomplete warm-up.
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate products: %w", err)
	}

	log.Printf("stock counters: %d loaded, %d live counter(s) kept", result.Loaded, result.Kept)
	return result, nil
}
