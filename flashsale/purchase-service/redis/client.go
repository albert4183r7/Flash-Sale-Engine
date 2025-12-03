package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps Redis client with flash sale specific operations
type Client struct {
	rdb *redis.Client
}

// NewClient creates a new Redis client
func NewClient(addr, password string) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Could not connect to Redis: %v", err)
	} else {
		log.Println("Connected to Redis successfully")
	}

	return &Client{rdb: rdb}
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.rdb.Close()
}

// SetIdempotencyKey sets an idempotency key with TTL
// Returns true if the key was set (first request), false if it already exists
func (c *Client) SetIdempotencyKey(ctx context.Context, userID, productID int, ttlSeconds int) (bool, error) {
	key := fmt.Sprintf("purchase:%d:%d", userID, productID)
	ttl := time.Duration(ttlSeconds) * time.Second

	result, err := c.rdb.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to set idempotency key: %w", err)
	}

	return result, nil
}

// InitializeStock sets initial stock for a product
func (c *Client) InitializeStock(ctx context.Context, productID, stock int) error {
	key := fmt.Sprintf("product_stock:%d", productID)
	return c.rdb.Set(ctx, key, stock, 0).Err()
}

// DecrementStock atomically decrements stock and returns the new value
// Returns the new stock value, or -1 if stock is already 0
func (c *Client) DecrementStock(ctx context.Context, productID, qty int) (int64, error) {
	key := fmt.Sprintf("product_stock:%d", productID)

	script := redis.NewScript(`
		local stock = redis.call('GET', KEYS[1])
		if stock == false then
			return -2
		end
		stock = tonumber(stock)
		local qty = tonumber(ARGV[1])
		if stock >= qty then
			return redis.call('DECRBY', KEYS[1], qty)
		else
			return -1
		end
	`)

	result, err := script.Run(ctx, c.rdb, []string{key}, qty).Int64()
	if err != nil {
		return -1, fmt.Errorf("failed to decrement stock: %w", err)
	}

	return result, nil
}

// GetStock returns the current stock for a product
func (c *Client) GetStock(ctx context.Context, productID int) (int64, error) {
	key := fmt.Sprintf("product_stock:%d", productID)
	result, err := c.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return -1, nil
	}
	if err != nil {
		return -1, fmt.Errorf("failed to get stock: %w", err)
	}
	return result, nil
}
