package models

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// OrderStatus is the lifecycle state of an order.
type OrderStatus string

const (
	// OrderStatusPending means the order has been accepted and queued but not
	// yet persisted by the order worker.
	OrderStatusPending OrderStatus = "PENDING"
	// OrderStatusSuccess means the order was persisted and stock was deducted.
	OrderStatusSuccess OrderStatus = "SUCCESS"
	// OrderStatusFailed means the order could not be fulfilled, typically
	// because the durable stock in PostgreSQL was insufficient.
	OrderStatusFailed OrderStatus = "FAILED"
	// OrderStatusCancelled means the buyer cancelled the order and stock was
	// returned to both Redis and PostgreSQL.
	OrderStatusCancelled OrderStatus = "CANCELLED"
)

// CancellableStatuses are the states that still hold stock, so cancelling an
// order in one of them returns its units. FAILED orders never took stock and
// CANCELLED ones have already given it back.
var CancellableStatuses = []OrderStatus{OrderStatusPending, OrderStatusSuccess}

// Cancellable reports whether an order in this state may be cancelled.
func (s OrderStatus) Cancellable() bool {
	return slices.Contains(CancellableStatuses, s)
}

// Order is a purchase record persisted in PostgreSQL.
type Order struct {
	ID        uuid.UUID   `json:"id"`
	UserID    uuid.UUID   `json:"user_id"`
	ProductID uuid.UUID   `json:"product_id"`
	Qty       int         `json:"qty"`
	Status    OrderStatus `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}

// OrderEvent is the message contract published by the purchase service and
// consumed by the order worker. It is defined once here so the producer and
// consumer cannot drift apart.
type OrderEvent struct {
	OrderID   uuid.UUID `json:"order_id"`
	UserID    uuid.UUID `json:"user_id"`
	ProductID uuid.UUID `json:"product_id"`
	Qty       int       `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}

// Validate reports whether the event carries the fields required to persist an
// order. Messages that fail validation can never succeed and must not be
// requeued.
func (e OrderEvent) Validate() error {
	switch {
	case e.OrderID == uuid.Nil:
		return errInvalidEvent("order_id is required")
	case e.UserID == uuid.Nil:
		return errInvalidEvent("user_id is required")
	case e.ProductID == uuid.Nil:
		return errInvalidEvent("product_id is required")
	case e.Qty <= 0:
		return errInvalidEvent("qty must be greater than zero")
	}
	return nil
}

type errInvalidEvent string

func (e errInvalidEvent) Error() string { return "invalid order event: " + string(e) }
