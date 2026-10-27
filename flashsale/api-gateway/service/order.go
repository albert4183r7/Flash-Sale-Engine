package service

import (
	"context"
	"errors"
	"log"

	"github.com/flashsale/api-gateway/repository"
	"github.com/flashsale/common/models"
	"github.com/google/uuid"
)

// Order lookup failures. The repository's are re-exported so handlers depend
// only on this package.
var (
	ErrOrderNotFound       = repository.ErrOrderNotFound
	ErrOrderNotCancellable = repository.ErrOrderNotCancellable
	// ErrOrderPending means the order was accepted but the worker has not
	// stored it yet, so it cannot be cancelled until it has.
	ErrOrderPending = errors.New("order is still being processed")
)

// OrderStore is the order storage the order logic needs.
type OrderStore interface {
	FindByIDForUser(ctx context.Context, orderID, userID uuid.UUID) (models.Order, error)
	CancelForUser(ctx context.Context, orderID, userID uuid.UUID) (models.Order, error)
}

// StockRestorer returns cancelled units to the in-memory stock counter.
type StockRestorer interface {
	RestoreStock(ctx context.Context, productID uuid.UUID, qty int) error
}

// PendingOrders reads orders that are accepted but not yet stored.
type PendingOrders interface {
	PendingOrder(ctx context.Context, orderID uuid.UUID) (models.OrderEvent, bool, error)
}

// Order serves order lookups and cancellations.
type Order struct {
	orders  OrderStore
	stock   StockRestorer
	pending PendingOrders
}

// NewOrder creates an Order service.
func NewOrder(orders OrderStore, stock StockRestorer, pending PendingOrders) *Order {
	return &Order{orders: orders, stock: stock, pending: pending}
}

// Get returns the caller's own order, or ErrOrderNotFound.
//
// An order reaches PostgreSQL only once the worker has stored it, a moment
// after the purchase is accepted. Until then it is read from the purchase
// service and reported as PENDING, so a buyer checking straight after buying
// does not see their order as missing.
func (o *Order) Get(ctx context.Context, orderID, userID uuid.UUID) (models.Order, error) {
	order, err := o.orders.FindByIDForUser(ctx, orderID, userID)
	if !errors.Is(err, ErrOrderNotFound) {
		return order, err
	}

	event, ok := o.pendingFor(ctx, orderID, userID)
	if !ok {
		return models.Order{}, ErrOrderNotFound
	}
	return models.Order{
		ID:        event.OrderID,
		UserID:    event.UserID,
		ProductID: event.ProductID,
		Qty:       event.Qty,
		Status:    models.OrderStatusPending,
		CreatedAt: event.Timestamp,
	}, nil
}

// pendingFor returns the caller's own pending order, if there is one. A lookup
// failure is logged and treated as no pending order: the order is then simply
// reported missing, as it was before pending orders were readable.
func (o *Order) pendingFor(ctx context.Context, orderID, userID uuid.UUID) (models.OrderEvent, bool) {
	event, found, err := o.pending.PendingOrder(ctx, orderID)
	if err != nil {
		log.Printf("pending order lookup for %s failed: %v", orderID, err)
		return models.OrderEvent{}, false
	}
	// Another buyer's order must look exactly like a missing one.
	if !found || event.UserID != userID {
		return models.OrderEvent{}, false
	}
	return event, true
}

// Cancel cancels the caller's own order and returns its stock.
//
// PostgreSQL is updated first because it is the authoritative record and the
// only place the cancellation can be made atomic and exactly-once. Only after
// it commits is the in-memory counter credited. Doing it the other way round
// would credit Redis for a cancellation that then failed to commit, and the
// flash sale would oversell.
func (o *Order) Cancel(ctx context.Context, orderID, userID uuid.UUID) (models.Order, error) {
	cancelled, err := o.orders.CancelForUser(ctx, orderID, userID)
	if errors.Is(err, ErrOrderNotFound) {
		if _, pending := o.pendingFor(ctx, orderID, userID); pending {
			return models.Order{}, ErrOrderPending
		}
	}
	if err != nil {
		return models.Order{}, err
	}

	if err := o.stock.RestoreStock(ctx, cancelled.ProductID, cancelled.Qty); err != nil {
		// The cancellation itself succeeded and durable stock is correct, so the
		// buyer is not told it failed. The in-memory counter is now lower than
		// the durable one, which under-sells rather than oversells and is
		// corrected at the next warm-up.
		log.Printf("CRITICAL: order %s cancelled but %d units of product %s were not returned to the stock cache: %v",
			cancelled.ID, cancelled.Qty, cancelled.ProductID, err)
	}

	return cancelled, nil
}
