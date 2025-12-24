// Package redis provides Redis client operations for stock management and idempotency.
//
// This package implements atomic stock operations using Lua scripts to ensure
// thread-safety in high-concurrency flash sale scenarios. It also provides
// idempotency key management to prevent duplicate purchases.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Client wraps the Redis client with flash sale specific operations.
type Client struct {
	rdb *redis.Client
}

// NewClient creates a new Redis client connected to the specified address.
// Parameters:
//   - addr: Redis server address (e.g., "localhost:6379")
//   - password: Redis password (empty string if no auth)
func NewClient(addr, password string) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	return &Client{rdb: rdb}
}

// Close gracefully closes the Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// SetIdempotencyKey creates an idempotency key to prevent duplicate purchases.
// Returns true if this is the first request (key was set), false if duplicate.
//
// Key format: "purchase:{userID}:{productID}"
// TTL: configurable, typically 2 minutes to allow legitimate retry after failure
//
// Example:
//
//	isFirst, err := client.SetIdempotencyKey(ctx, userID, productID, 120)
//	if !isFirst {
//	    return "Duplicate purchase"
//	}
func (c *Client) SetIdempotencyKey(ctx context.Context, userID, productID uuid.UUID, ttlSeconds int) (bool, error) {
	key := fmt.Sprintf("purchase:%s:%s", userID.String(), productID.String())
	result, err := c.rdb.SetNX(ctx, key, "1", time.Duration(ttlSeconds)*time.Second).Result()
	return result, err
}

// InitializeStock sets the initial stock for a product in Redis.
// This should be called during system warmup to sync Redis with database.
//
// Key format: "stock:{productID}"
func (c *Client) InitializeStock(ctx context.Context, productID uuid.UUID, stock int) error {
	key := fmt.Sprintf("stock:%s", productID.String())
	return c.rdb.Set(ctx, key, stock, 0).Err()
}

// DecrementStock atomically decrements stock using a Lua script.
// This ensures thread-safety even with thousands of concurrent requests.
//
// Returns:
//   - >= 0: New stock count after decrement (success)
//   - -1: Insufficient stock (out of stock)
//   - -2: Product not found (stock key doesn't exist)
//
// The Lua script ensures atomicity:
//  1. Check if stock exists
//  2. Check if enough stock available
//  3. Decrement and return new value
//
// All three steps happen as a single atomic operation.
func (c *Client) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) (int64, error) {
	key := fmt.Sprintf("stock:%s", productID.String())

	// Lua script for atomic stock decrement
	// KEYS[1] = stock key
	// ARGV[1] = quantity to decrement
	script := redis.NewScript(`
		local stock = tonumber(redis.call('GET', KEYS[1]))
		if stock == nil then return -2 end
		local qty = tonumber(ARGV[1])
		if stock >= qty then
			return redis.call('DECRBY', KEYS[1], qty)
		else
			return -1
		end
	`)
	return script.Run(ctx, c.rdb, []string{key}, qty).Int64()
}

// IncrementStock atomically increments stock (used for order cancellation).
// This restores stock when an order is cancelled or fails.
func (c *Client) IncrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	key := fmt.Sprintf("stock:%s", productID.String())
	return c.rdb.IncrBy(ctx, key, int64(qty)).Err()
}

// GetStock returns the current stock for a product.
// Returns -1 if the product doesn't exist in Redis.
func (c *Client) GetStock(ctx context.Context, productID uuid.UUID) (int64, error) {
	key := fmt.Sprintf("stock:%s", productID.String())
	val, err := c.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return -1, nil
	}
	return val, err
}