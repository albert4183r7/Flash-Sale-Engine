// Package redis holds the flash sale's live state: the authoritative stock
// counters, the idempotency claims that stop a buyer double-ordering, the
// outbox of order events awaiting publication, and pending copies of orders
// the worker has not stored yet.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/flashsale/common/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Errors returned by the stock operations. They replace the sentinels the Lua
// scripts use on the wire, so callers never compare against magic values.
var (
	// ErrProductUnknown means no stock counter exists for the product, which
	// happens when the product was created after stock was last loaded.
	ErrProductUnknown = errors.New("product stock not initialised")
	// ErrOutOfStock means the remaining stock cannot cover the request.
	ErrOutOfStock = errors.New("insufficient stock")
	// ErrNotPending means no pending order is held for the ID.
	ErrNotPending = errors.New("order is not pending")
)

// Outcomes reported by reserveScript.
const (
	outcomeReserved       = "RESERVED"
	outcomeDuplicate      = "DUPLICATE"
	outcomeOutOfStock     = "OUT_OF_STOCK"
	outcomeUnknownProduct = "UNKNOWN_PRODUCT"
)

// reserveScript places an order in one atomic step. Redis runs a script as a
// single unit, so either every write below happens or none does:
//
//  1. If the buyer already holds a claim on this product, return the order ID
//     stored in it, so a retried request gets its original order back.
//  2. Check and decrement stock, so concurrent buyers can never oversell.
//  3. Store the claim, holding the new order ID, for the idempotency window.
//  4. Add the order event to the outbox, from which it is published. An event
//     whose publish is interrupted, even by a crash, stays there to be retried.
//  5. Keep a pending copy of the order, readable until the worker stores it.
//
// KEYS: claim, stock, outbox, pending. ARGV: order ID, qty, claim TTL (ms),
// event JSON, pending TTL (ms). A script may only touch keys on one node, so a
// move to Redis Cluster would need these keys to share a hash tag.
var reserveScript = redis.NewScript(`
	local existing = redis.call('GET', KEYS[1])
	if existing then return {'DUPLICATE', existing} end

	local stock = tonumber(redis.call('GET', KEYS[2]))
	if stock == nil then return {'UNKNOWN_PRODUCT'} end
	local qty = tonumber(ARGV[2])
	if stock < qty then return {'OUT_OF_STOCK'} end

	local remaining = redis.call('DECRBY', KEYS[2], qty)
	redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
	redis.call('HSET', KEYS[3], ARGV[1], ARGV[4])
	redis.call('SET', KEYS[4], ARGV[4], 'PX', ARGV[5])
	return {'RESERVED', tostring(remaining)}
`)

// releaseScript undoes a reservation whose event could not be published. It
// acts only if the event is still in the outbox, which makes it safe to call
// more than once and stops it racing the outbox relay: whichever removes the
// entry first owns the event.
//
// KEYS: outbox, stock, claim, pending. ARGV: order ID, qty.
var releaseScript = redis.NewScript(`
	if redis.call('HDEL', KEYS[1], ARGV[1]) == 0 then return 0 end
	redis.call('INCRBY', KEYS[2], ARGV[2])
	if redis.call('GET', KEYS[3]) == ARGV[1] then redis.call('DEL', KEYS[3]) end
	redis.call('DEL', KEYS[4])
	return 1
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

func pendingKey(orderID uuid.UUID) string {
	return "order:pending:" + orderID.String()
}

// outboxKey is the hash of order events reserved but not yet confirmed by the
// broker, keyed by order ID.
const outboxKey = "orders:outbox"

// Reservation is one order to place atomically.
type Reservation struct {
	Event models.OrderEvent
	// ClaimTTL is how long the buyer's claim on the product is held, which is
	// the window in which a resubmission returns the same order.
	ClaimTTL time.Duration
	// PendingTTL is how long the pending copy of the order stays readable. It
	// only needs to outlast the time the worker takes to store the order.
	PendingTTL time.Duration
}

// Reserved is the result of a successful Reserve.
type Reserved struct {
	// OrderID is the new order's ID or, for a duplicate, the original one.
	OrderID uuid.UUID
	// Duplicate reports that the buyer already had an order for this product
	// within the claim window, and nothing new was reserved.
	Duplicate bool
	// Remaining is the stock left after a new reservation.
	Remaining int64
}

// Reserve claims, reserves stock for and records an order in one atomic step;
// see reserveScript. It returns ErrOutOfStock or ErrProductUnknown when the
// order cannot be placed, in which case nothing was written.
func (c *Client) Reserve(ctx context.Context, r Reservation) (Reserved, error) {
	e := r.Event
	payload, err := json.Marshal(e)
	if err != nil {
		return Reserved{}, fmt.Errorf("encode order %s: %w", e.OrderID, err)
	}

	keys := []string{idempotencyKey(e.UserID, e.ProductID), stockKey(e.ProductID), outboxKey, pendingKey(e.OrderID)}
	reply, err := reserveScript.Run(ctx, c.rdb, keys,
		e.OrderID.String(), e.Qty, r.ClaimTTL.Milliseconds(), payload, r.PendingTTL.Milliseconds()).StringSlice()
	if err != nil {
		return Reserved{}, fmt.Errorf("reserve order %s: %w", e.OrderID, err)
	}

	switch reply[0] {
	case outcomeReserved:
		remaining, err := strconv.ParseInt(reply[1], 10, 64)
		if err != nil {
			return Reserved{}, fmt.Errorf("parse remaining stock %q: %w", reply[1], err)
		}
		return Reserved{OrderID: e.OrderID, Remaining: remaining}, nil
	case outcomeDuplicate:
		original, err := uuid.Parse(reply[1])
		if err != nil {
			return Reserved{}, fmt.Errorf("parse claimed order id %q: %w", reply[1], err)
		}
		return Reserved{OrderID: original, Duplicate: true}, nil
	case outcomeOutOfStock:
		return Reserved{}, ErrOutOfStock
	case outcomeUnknownProduct:
		return Reserved{}, ErrProductUnknown
	default:
		return Reserved{}, fmt.Errorf("reserve order %s: unexpected outcome %q", e.OrderID, reply[0])
	}
}

// Release undoes the reservation for an event that could not be published:
// the stock is returned and the claim and pending copy are dropped. It reports
// false when the event had already left the outbox, in which case nothing is
// changed.
func (c *Client) Release(ctx context.Context, e models.OrderEvent) (bool, error) {
	keys := []string{outboxKey, stockKey(e.ProductID), idempotencyKey(e.UserID, e.ProductID), pendingKey(e.OrderID)}
	released, err := releaseScript.Run(ctx, c.rdb, keys, e.OrderID.String(), e.Qty).Int()
	if err != nil {
		return false, fmt.Errorf("release order %s: %w", e.OrderID, err)
	}
	return released == 1, nil
}

// MarkPublished removes an event from the outbox once the broker has confirmed
// it.
func (c *Client) MarkPublished(ctx context.Context, orderID uuid.UUID) error {
	if err := c.rdb.HDel(ctx, outboxKey, orderID.String()).Err(); err != nil {
		return fmt.Errorf("remove order %s from outbox: %w", orderID, err)
	}
	return nil
}

// StaleOutbox returns the outbox events reserved more than olderThan ago. An
// event that old was not confirmed by the request that reserved it, because
// that request crashed or lost its connection, and must be published again.
func (c *Client) StaleOutbox(ctx context.Context, olderThan time.Duration) ([]models.OrderEvent, error) {
	entries, err := c.rdb.HGetAll(ctx, outboxKey).Result()
	if err != nil {
		return nil, fmt.Errorf("read outbox: %w", err)
	}

	cutoff := time.Now().Add(-olderThan)
	var stale []models.OrderEvent
	for id, payload := range entries {
		var e models.OrderEvent
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			return nil, fmt.Errorf("decode outbox entry %s: %w", id, err)
		}
		if e.Timestamp.Before(cutoff) {
			stale = append(stale, e)
		}
	}
	return stale, nil
}

// PendingOrder returns an order that has been accepted but may not be stored
// by the worker yet, or ErrNotPending when none is held for the ID.
func (c *Client) PendingOrder(ctx context.Context, orderID uuid.UUID) (models.OrderEvent, error) {
	payload, err := c.rdb.Get(ctx, pendingKey(orderID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return models.OrderEvent{}, ErrNotPending
	}
	if err != nil {
		return models.OrderEvent{}, fmt.Errorf("read pending order %s: %w", orderID, err)
	}

	var e models.OrderEvent
	if err := json.Unmarshal(payload, &e); err != nil {
		return models.OrderEvent{}, fmt.Errorf("decode pending order %s: %w", orderID, err)
	}
	return e, nil
}

// InitializeStock seeds the counter for a product from the durable stock value,
// unless a counter already exists. A live counter already accounts for orders
// still on their way to the database, which the durable value does not yet
// reflect, so overwriting it would let the sale oversell. It reports whether
// the counter was created.
func (c *Client) InitializeStock(ctx context.Context, productID uuid.UUID, stock int) (bool, error) {
	created, err := c.rdb.SetNX(ctx, stockKey(productID), stock, 0).Result()
	if err != nil {
		return false, fmt.Errorf("initialise stock for product %s: %w", productID, err)
	}
	return created, nil
}

// IncrementStock returns qty units to the counter, used when an order is
// cancelled.
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
