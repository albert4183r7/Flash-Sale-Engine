// Package repository contains the order worker's PostgreSQL data access.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/flashsale/common/models"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ErrAlreadyPersisted reports that an order with the same ID is already stored.
// RabbitMQ guarantees at-least-once delivery, so redeliveries are expected and
// must be treated as success rather than as an error.
var ErrAlreadyPersisted = errors.New("order already persisted")

// ErrPermanent wraps a failure that can never succeed on redelivery, such as an
// order referencing a user or product that does not exist. Messages that fail
// this way must be discarded instead of requeued, otherwise they spin forever.
var ErrPermanent = errors.New("permanent failure")

// OrderRepository persists orders and keeps durable product stock in step.
type OrderRepository struct {
	db *sql.DB
}

// NewOrderRepository creates an OrderRepository backed by db.
func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// Persist stores the order and deducts durable stock in a single transaction,
// so an order is never recorded as successful without the matching stock
// deduction. It returns the status the order was stored with.
//
// When the durable stock in PostgreSQL cannot cover the order the order is
// recorded as FAILED rather than silently succeeding, which makes the
// divergence between Redis and PostgreSQL visible for reconciliation.
func (r *OrderRepository) Persist(ctx context.Context, event models.OrderEvent) (models.OrderStatus, error) {
	if err := event.Validate(); err != nil {
		return "", fmt.Errorf("%w: %w", ErrPermanent, err)
	}

	status, err := r.persistWithStatus(ctx, event, models.OrderStatusSuccess, true)
	if err == nil {
		return status, nil
	}
	if !errors.Is(err, errInsufficientStock) {
		return "", err
	}

	// Durable stock could not cover the order. Record the attempt as FAILED so
	// the order is not lost and the mismatch can be reconciled.
	if _, err := r.persistWithStatus(ctx, event, models.OrderStatusFailed, false); err != nil {
		return "", err
	}
	return models.OrderStatusFailed, nil
}

var errInsufficientStock = errors.New("insufficient durable stock")

// persistWithStatus inserts the order with the given status, optionally
// deducting stock in the same transaction.
func (r *OrderRepository) persistWithStatus(
	ctx context.Context,
	event models.OrderEvent,
	status models.OrderStatus,
	deductStock bool,
) (models.OrderStatus, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}
	// Rollback is a no-op once the transaction has been committed.
	defer func() { _ = tx.Rollback() }()

	// ON CONFLICT DO NOTHING makes redelivery of the same order harmless.
	const insertOrder = `
		INSERT INTO orders (id, user_id, product_id, qty, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO NOTHING`

	res, err := tx.ExecContext(ctx, insertOrder,
		event.OrderID, event.UserID, event.ProductID, event.Qty, status, event.Timestamp)
	if err != nil {
		return "", classify(fmt.Errorf("insert order %s: %w", event.OrderID, err))
	}

	inserted, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("read insert result: %w", err)
	}
	if inserted == 0 {
		return "", ErrAlreadyPersisted
	}

	if deductStock {
		// The stock guard keeps the products.stock >= 0 constraint satisfied and
		// tells us, without racing, whether the deduction actually happened.
		const deduct = `UPDATE products SET stock = stock - $1 WHERE id = $2 AND stock >= $1`

		res, err := tx.ExecContext(ctx, deduct, event.Qty, event.ProductID)
		if err != nil {
			return "", classify(fmt.Errorf("deduct stock for product %s: %w", event.ProductID, err))
		}
		updated, err := res.RowsAffected()
		if err != nil {
			return "", fmt.Errorf("read stock update result: %w", err)
		}
		if updated == 0 {
			return "", errInsufficientStock
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit transaction: %w", err)
	}
	return status, nil
}

// FindStatus returns the stored status of an order, used by the integration
// tests and for operational inspection.
func (r *OrderRepository) FindStatus(ctx context.Context, orderID uuid.UUID) (models.OrderStatus, error) {
	var status models.OrderStatus
	err := r.db.QueryRowContext(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status)
	if err != nil {
		return "", err
	}
	return status, nil
}

// classify marks constraint violations as permanent. A foreign key or check
// violation means the message references data that does not exist, so retrying
// it can only fail again.
func classify(err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code.Class() {
		case "23": // integrity constraint violation
			return fmt.Errorf("%w: %w", ErrPermanent, err)
		}
	}
	return err
}
