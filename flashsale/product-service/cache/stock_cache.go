package cache

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// StockCache handles Redis-based stock operations with atomic Lua scripts
type StockCache struct {
	client *redis.Client
}

func NewStockCache(addr, password string) *StockCache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	return &StockCache{client: client}
}

func (c *StockCache) Close() error {
	return c.client.Close()
}

func (c *StockCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *StockCache) stockKey(productID uuid.UUID) string {
	return fmt.Sprintf("stock:%s", productID.String())
}

// InitializeStock sets the stock for a product in Redis
func (c *StockCache) InitializeStock(ctx context.Context, productID uuid.UUID, stock int) error {
	return c.client.Set(ctx, c.stockKey(productID), stock, 0).Err()
}

// GetStock returns the current stock from Redis
func (c *StockCache) GetStock(ctx context.Context, productID uuid.UUID) (int, error) {
	val, err := c.client.Get(ctx, c.stockKey(productID)).Int()
	if err == redis.Nil {
		return -1, nil // Product not found
	}
	return val, err
}

// DecrementStock atomically decrements stock using Lua script
// Returns: new stock value, or -1 if insufficient, or -2 if product not found
func (c *StockCache) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) (int, error) {
	script := redis.NewScript(`
		local stock = redis.call('GET', KEYS[1])
		if stock == false then
			return -2
		end
		stock = tonumber(stock)
		if stock < tonumber(ARGV[1]) then
			return -1
		end
		return redis.call('DECRBY', KEYS[1], ARGV[1])
	`)

	result, err := script.Run(ctx, c.client, []string{c.stockKey(productID)}, qty).Int()
	if err != nil {
		return 0, err
	}
	return result, nil
}

// IncrementStock atomically increments stock (for restoring cancelled orders)
func (c *StockCache) IncrementStock(ctx context.Context, productID uuid.UUID, qty int) (int, error) {
	val, err := c.client.IncrBy(ctx, c.stockKey(productID), int64(qty)).Result()
	return int(val), err
}

// SetIdempotencyKey sets a key to prevent duplicate purchases
func (c *StockCache) SetIdempotencyKey(ctx context.Context, userID, productID uuid.UUID, ttlSeconds int) (bool, error) {
	key := fmt.Sprintf("idem:%s:%s", userID.String(), productID.String())
	result, err := c.client.SetNX(ctx, key, "1", 0).Result()
	if err != nil {
		return false, err
	}
	if result {
		// Set TTL separately
		c.client.Expire(ctx, key, redis.KeepTTL)
	}
	return result, nil
}
