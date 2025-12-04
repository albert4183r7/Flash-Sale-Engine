package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func NewClient(addr, password string) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	return &Client{rdb: rdb}
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

// SetIdempotencyKey uses UUIDs for key generation
func (c *Client) SetIdempotencyKey(ctx context.Context, userID, productID uuid.UUID, ttlSeconds int) (bool, error) {
	key := fmt.Sprintf("purchase:%s:%s", userID.String(), productID.String())
	result, err := c.rdb.SetNX(ctx, key, "1", time.Duration(ttlSeconds)*time.Second).Result()
	return result, err
}

func (c *Client) InitializeStock(ctx context.Context, productID uuid.UUID, stock int) error {
	key := fmt.Sprintf("product_stock:%s", productID.String())
	return c.rdb.Set(ctx, key, stock, 0).Err()
}

func (c *Client) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) (int64, error) {
	key := fmt.Sprintf("product_stock:%s", productID.String())
	// Lua script
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

func (c *Client) IncrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	key := fmt.Sprintf("product_stock:%s", productID.String())
	return c.rdb.IncrBy(ctx, key, int64(qty)).Err()
}

func (c *Client) GetStock(ctx context.Context, productID uuid.UUID) (int64, error) {
	key := fmt.Sprintf("product_stock:%s", productID.String())
	val, err := c.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return -1, nil
	}
	return val, err
}