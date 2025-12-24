package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type UserCache struct {
	client *redis.Client
}

type CachedUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func NewUserCache(addr, password string) *UserCache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	return &UserCache{client: client}
}

func (c *UserCache) Close() error {
	return c.client.Close()
}

func (c *UserCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *UserCache) GetUser(ctx context.Context, email string) (*CachedUser, error) {
	key := fmt.Sprintf("user:%s", email)
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var user CachedUser
	if err := json.Unmarshal([]byte(val), &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *UserCache) SetUser(ctx context.Context, email string, user *CachedUser, ttl time.Duration) error {
	key := fmt.Sprintf("user:%s", email)
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *UserCache) InvalidateUser(ctx context.Context, email string) error {
	key := fmt.Sprintf("user:%s", email)
	return c.client.Del(ctx, key).Err()
}
