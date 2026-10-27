// Package redis holds the flash sale's authoritative in-memory stock counters
// and the short-lived idempotency keys that stop a buyer double-ordering.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Errors returned by the stock operations. They replace the numeric sentinels
// the Lua script uses on the wire, so callers never compare against magic
// numbers.
var (
	// ErrProductUnknown means no stock counter exists for the product, which
	// happens when the product was created after the service warmed up.
	ErrProductUnknown = errors.New("product stock not initialised")
	// ErrOutOfStock means the remaining stock cannot cover the request.
	ErrOutOfStock = errors.New("insufficient stock")
)

// Lua sentinels returned by decrementScript.
const (
	sentinelUnknownProduct = -2
	sentinelOutOfStock     = -1
)

// decrementScript checks and decrements stock atomically. Redis runs the whole
// script as one unit, so concurrent buyers can never oversell.
var decrementScript = redis.NewScript(`
	local stock = tonumber(redis.call('GET', KEYS[1]))
	if stock == nil then return -2 end
	local qty = tonumber(ARGV[1])
	if stock >= qty then
		return redis.call('DECRBY', KEYS[1], qty)
	end
	return -1
`)

// Client wraps the Redis connection used by the purchase service.
type Client struct {
	rdb *redis.Client
}

// NewClient creates a Redis client for the given address.
func NewClient(addr, password string) *Client {
	return &Client{rdb: redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})}
}

// Close releases the connection pool.
func (c *Client) Close() error { return c.rdb.Close() }

// Ping verifies that Redis is reachable.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	return nil
}

func stockKey(productID uuid.UUID) string {
	return "product_stock:" + productID.String()
}

func idempotencyKey(userID, productID uuid.UUID) string {
	return fmt.Sprintf("purchase:%s:%s", userID, productID)
}

// ClaimPurchase reserves the buyer's right to order this product. It reports
// false when a claim is already held, which is how repeated submissions of the
// same order are rejected.
func (c *Client) ClaimPurchase(ctx context.Context, userID, productID uuid.UUID, ttl time.Duration) (bool, error) {
	ok, err := c.rdb.SetNX(ctx, idempotencyKey(userID, productID), "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("claim purchase: %w", err)
	}
	return ok, nil
}

// ReleasePurchaseClaim drops the claim taken by ClaimPurchase. It is used to
// compensate a failed purchase so the buyer can retry immediately instead of
// waiting out the idempotency window.
func (c *Client) ReleasePurchaseClaim(ctx context.Context, userID, productID uuid.UUID) error {
	if err := c.rdb.Del(ctx, idempotencyKey(userID, productID)).Err(); err != nil {
		return fmt.Errorf("release purchase claim: %w", err)
	}
	return nil
}

// InitializeStock seeds the counter for a product from the durable stock value.
func (c *Client) InitializeStock(ctx context.Context, productID uuid.UUID, stock int) error {
	if err := c.rdb.Set(ctx, stockKey(productID), stock, 0).Err(); err != nil {
		return fmt.Errorf("initialise stock for product %s: %w", productID, err)
	}
	return nil
}

// DecrementStock atomically reserves qty units and returns the remaining stock.
// It returns ErrOutOfStock or ErrProductUnknown when the reservation is refused.
func (c *Client) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) (int64, error) {
	remaining, err := decrementScript.Run(ctx, c.rdb, []string{stockKey(productID)}, qty).Int64()
	if err != nil {
		return 0, fmt.Errorf("decrement stock for product %s: %w", productID, err)
	}

	switch remaining {
	case sentinelUnknownProduct:
		return 0, ErrProductUnknown
	case sentinelOutOfStock:
		return 0, ErrOutOfStock
	}
	return remaining, nil
}

// IncrementStock returns qty units to the counter, used when a reservation is
// compensated or an order is cancelled.
func (c *Client) IncrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	if err := c.rdb.IncrBy(ctx, stockKey(productID), int64(qty)).Err(); err != nil {
		return fmt.Errorf("increment stock for product %s: %w", productID, err)
	}
	return nil
}

// GetStock returns the current stock for a product, or ErrProductUnknown when
// no counter exists.
func (c *Client) GetStock(ctx context.Context, productID uuid.UUID) (int64, error) {
	stock, err := c.rdb.Get(ctx, stockKey(productID)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, ErrProductUnknown
	}
	if err != nil {
		return 0, fmt.Errorf("get stock for product %s: %w", productID, err)
	}
	return stock, nil
}
