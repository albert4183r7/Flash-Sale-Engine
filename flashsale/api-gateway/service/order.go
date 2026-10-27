package service

import (
	"context"
	"log"

	"github.com/flashsale/api-gateway/repository"
	"github.com/flashsale/common/models"
	"github.com/google/uuid"
)

// Order lookup failures, re-exported so handlers depend only on this package.
var (
	ErrOrderNotFound       = repository.ErrOrderNotFound
	ErrOrderNotCancellable = repository.ErrOrderNotCancellable
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

// Order serves order lookups and cancellations.
type Order struct {
	orders OrderStore
	stock  StockRestorer
}

// NewOrder creates an Order service.
func NewOrder(orders OrderStore, stock StockRestorer) *Order {
	return &Order{orders: orders, stock: stock}
}

// Get returns the caller's own order, or ErrOrderNotFound.
func (o *Order) Get(ctx context.Context, orderID, userID uuid.UUID) (models.Order, error) {
	return o.orders.FindByIDForUser(ctx, orderID, userID)
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
