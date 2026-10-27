// Package service holds the flash sale's purchase logic: reserve stock in
// Redis, then hand the order off to RabbitMQ for durable processing.
package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/purchase-service/redis"
	"github.com/google/uuid"
)

// Errors a purchase can fail with. Callers map these to HTTP status codes.
var (
	// ErrDuplicatePurchase means the buyer already has a claim on this product
	// within the idempotency window.
	ErrDuplicatePurchase = errors.New("duplicate purchase request")
	// ErrOutOfStock means the remaining stock cannot cover the request.
	ErrOutOfStock = errors.New("product is out of stock")
	// ErrProductUnknown means the product has no stock counter.
	ErrProductUnknown = errors.New("product not found")
)

// StockStore is the stock and idempotency backend the purchase logic needs.
// Defining it here, where it is consumed, keeps the service testable without a
// live Redis.
type StockStore interface {
	ClaimPurchase(ctx context.Context, userID, productID uuid.UUID, ttl time.Duration) (bool, error)
	ReleasePurchaseClaim(ctx context.Context, userID, productID uuid.UUID) error
	DecrementStock(ctx context.Context, productID uuid.UUID, qty int) (int64, error)
	IncrementStock(ctx context.Context, productID uuid.UUID, qty int) error
}

// EventPublisher hands an accepted order to the asynchronous pipeline.
type EventPublisher interface {
	PublishOrderCreated(event models.OrderEvent) error
}

// Purchase reserves stock and publishes order events.
type Purchase struct {
	stock          StockStore
	publisher      EventPublisher
	idempotencyTTL time.Duration
}

// NewPurchase creates a Purchase service.
func NewPurchase(stock StockStore, publisher EventPublisher, idempotencyTTL time.Duration) *Purchase {
	return &Purchase{stock: stock, publisher: publisher, idempotencyTTL: idempotencyTTL}
}

// Process reserves stock for the buyer and queues the order, returning the ID
// assigned to it.
//
// Reserving stock and publishing the event are two separate systems, so every
// failure after a successful reservation compensates by returning the stock and
// releasing the buyer's claim. Without that, a failed publish would burn
// inventory that no order will ever consume.
func (p *Purchase) Process(ctx context.Context, userID, productID uuid.UUID, qty int) (uuid.UUID, error) {
	if qty <= 0 {
		return uuid.Nil, fmt.Errorf("qty must be greater than zero, got %d", qty)
	}

	claimed, err := p.stock.ClaimPurchase(ctx, userID, productID, p.idempotencyTTL)
	if err != nil {
		return uuid.Nil, fmt.Errorf("claim purchase: %w", err)
	}
	if !claimed {
		return uuid.Nil, ErrDuplicatePurchase
	}

	remaining, err := p.stock.DecrementStock(ctx, productID, qty)
	if err != nil {
		// The claim is only meaningful alongside a reservation. Release it so a
		// buyer who hit an out-of-stock product is not locked out of retrying.
		p.releaseClaim(ctx, userID, productID)
		return uuid.Nil, translateStockError(err)
	}

	orderID := uuid.New()
	event := models.OrderEvent{
		OrderID:   orderID,
		UserID:    userID,
		ProductID: productID,
		Qty:       qty,
		Timestamp: time.Now().UTC(),
	}

	if err := p.publisher.PublishOrderCreated(event); err != nil {
		p.compensate(ctx, userID, productID, qty)
		return uuid.Nil, fmt.Errorf("queue order %s: %w", orderID, err)
	}

	log.Printf("order %s accepted for product %s, %d remaining", orderID, productID, remaining)
	return orderID, nil
}

// compensate returns reserved stock and releases the buyer's claim after the
// order could not be queued.
func (p *Purchase) compensate(ctx context.Context, userID, productID uuid.UUID, qty int) {
	if err := p.stock.IncrementStock(ctx, productID, qty); err != nil {
		// Log loudly: the reserved units are now unsellable until reconciled.
		log.Printf("CRITICAL: failed to return %d units of product %s after a failed publish: %v",
			qty, productID, err)
	}
	p.releaseClaim(ctx, userID, productID)
}

func (p *Purchase) releaseClaim(ctx context.Context, userID, productID uuid.UUID) {
	if err := p.stock.ReleasePurchaseClaim(ctx, userID, productID); err != nil {
		log.Printf("failed to release purchase claim for user %s on product %s: %v",
			userID, productID, err)
	}
}

// translateStockError converts storage-level failures into the service's own
// error vocabulary, so HTTP handlers depend only on this package. Both the
// service sentinels and the Redis ones are accepted, which lets tests supply a
// fake StockStore that speaks the service's vocabulary directly.
func translateStockError(err error) error {
	switch {
	case errors.Is(err, ErrOutOfStock), errors.Is(err, redis.ErrOutOfStock):
		return ErrOutOfStock
	case errors.Is(err, ErrProductUnknown), errors.Is(err, redis.ErrProductUnknown):
		return ErrProductUnknown
	default:
		return fmt.Errorf("reserve stock: %w", err)
	}
}
