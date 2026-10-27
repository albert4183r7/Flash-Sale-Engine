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
	// ErrOutOfStock means the remaining stock cannot cover the request.
	ErrOutOfStock = errors.New("product is out of stock")
	// ErrProductUnknown means the product has no stock counter.
	ErrProductUnknown = errors.New("product not found")
)

// StockStore is the reservation backend the purchase logic needs. Defining it
// here, where it is consumed, keeps the service testable without a live Redis.
type StockStore interface {
	Reserve(ctx context.Context, r redis.Reservation) (redis.Reserved, error)
	Release(ctx context.Context, event models.OrderEvent) (bool, error)
	MarkPublished(ctx context.Context, orderID uuid.UUID) error
	StaleOutbox(ctx context.Context, olderThan time.Duration) ([]models.OrderEvent, error)
}

// EventPublisher hands an accepted order to the asynchronous pipeline.
type EventPublisher interface {
	PublishOrderCreated(event models.OrderEvent) error
}

// TTLs controls how long a reservation's Redis state lives.
type TTLs struct {
	// Idempotency is how long a buyer's claim on a product is held: within it,
	// a resubmitted purchase returns the original order.
	Idempotency time.Duration
	// PendingOrder is how long an accepted order stays readable before the
	// worker has stored it.
	PendingOrder time.Duration
}

// Purchase reserves stock and publishes order events.
type Purchase struct {
	stock     StockStore
	publisher EventPublisher
	ttls      TTLs
}

// NewPurchase creates a Purchase service.
func NewPurchase(stock StockStore, publisher EventPublisher, ttls TTLs) *Purchase {
	return &Purchase{stock: stock, publisher: publisher, ttls: ttls}
}

// Placement is the outcome of an accepted purchase.
type Placement struct {
	OrderID uuid.UUID
	// Duplicate reports that the buyer had already ordered this product within
	// the idempotency window. OrderID is then that earlier order, and nothing
	// new was reserved.
	Duplicate bool
}

// Process reserves stock for the buyer and queues the order.
//
// The reservation, the idempotency claim and the outbox entry are written in
// one atomic step, so the order event exists in Redis before any attempt to
// publish it. If the publish fails, the reservation is released. If this
// process dies before it can do either, the event stays in the outbox and
// RepublishStale sends it later, so reserved stock is never stranded.
func (p *Purchase) Process(ctx context.Context, userID, productID uuid.UUID, qty int) (Placement, error) {
	if qty <= 0 {
		return Placement{}, fmt.Errorf("qty must be greater than zero, got %d", qty)
	}

	event := models.OrderEvent{
		OrderID:   uuid.New(),
		UserID:    userID,
		ProductID: productID,
		Qty:       qty,
		Timestamp: time.Now().UTC(),
	}

	reserved, err := p.stock.Reserve(ctx, redis.Reservation{
		Event:      event,
		ClaimTTL:   p.ttls.Idempotency,
		PendingTTL: p.ttls.PendingOrder,
	})
	switch {
	case errors.Is(err, redis.ErrOutOfStock):
		return Placement{}, ErrOutOfStock
	case errors.Is(err, redis.ErrProductUnknown):
		return Placement{}, ErrProductUnknown
	case err != nil:
		return Placement{}, err
	case reserved.Duplicate:
		return Placement{OrderID: reserved.OrderID, Duplicate: true}, nil
	}

	if err := p.publisher.PublishOrderCreated(event); err != nil {
		p.release(ctx, event)
		return Placement{}, fmt.Errorf("queue order %s: %w", event.OrderID, err)
	}
	p.markPublished(ctx, event.OrderID)

	log.Printf("order %s accepted for product %s, %d remaining", event.OrderID, productID, reserved.Remaining)
	return Placement{OrderID: event.OrderID}, nil
}

// RepublishStale publishes outbox events older than olderThan, which are events
// whose original request never confirmed them. olderThan must comfortably
// exceed the publisher's confirm timeout, so a request still waiting on the
// broker is not raced. A republished event may reach the worker twice; the
// worker stores each order ID once, so that is harmless. It returns how many
// events were published.
func (p *Purchase) RepublishStale(ctx context.Context, olderThan time.Duration) (int, error) {
	stale, err := p.stock.StaleOutbox(ctx, olderThan)
	if err != nil {
		return 0, err
	}

	var published int
	for _, event := range stale {
		if err := p.publisher.PublishOrderCreated(event); err != nil {
			// The broker is unavailable; the remaining events will be retried
			// on the next run.
			return published, fmt.Errorf("republish order %s: %w", event.OrderID, err)
		}
		p.markPublished(ctx, event.OrderID)
		published++
	}
	return published, nil
}

// release returns the stock and drops the claim of an order that could not be
// queued.
func (p *Purchase) release(ctx context.Context, event models.OrderEvent) {
	if _, err := p.stock.Release(ctx, event); err != nil {
		// The event is still in the outbox, so the relay will publish it and
		// the order will go through after all; nothing is lost.
		log.Printf("failed to release order %s after a failed publish, leaving it to the outbox relay: %v",
			event.OrderID, err)
	}
}

// markPublished takes a confirmed event out of the outbox.
func (p *Purchase) markPublished(ctx context.Context, orderID uuid.UUID) {
	if err := p.stock.MarkPublished(ctx, orderID); err != nil {
		// The relay will publish it again, which the worker ignores.
		log.Printf("failed to clear order %s from the outbox: %v", orderID, err)
	}
}
