package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/flashsale/common/models"
	"github.com/google/uuid"
)

// ErrOrderNotFound reports that no order matched the lookup. Orders are always
// looked up together with their owner, so this also covers an order that exists
// but belongs to somebody else: the caller must not be able to tell the two
// cases apart.
var ErrOrderNotFound = errors.New("order not found")

// ErrOrderNotCancellable reports that the order exists but is in a state that
// cannot be cancelled, such as one that was already cancelled.
var ErrOrderNotCancellable = errors.New("order cannot be cancelled")

// OrderRepository reads and updates orders.
type OrderRepository struct {
	db *sql.DB
}

// NewOrderRepository creates an OrderRepository backed by db.
func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// FindByIDForUser returns an order only if it belongs to the given user.
// Scoping the query by owner is what stops one buyer reading another's orders.
func (r *OrderRepository) FindByIDForUser(ctx context.Context, orderID, userID uuid.UUID) (models.Order, error) {
	const query = `
		SELECT id, user_id, product_id, qty, status, created_at
		FROM orders
		WHERE id = $1 AND user_id = $2`

	var o models.Order
	err := r.db.QueryRowContext(ctx, query, orderID, userID).
		Scan(&o.ID, &o.UserID, &o.ProductID, &o.Qty, &o.Status, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Order{}, ErrOrderNotFound
	}
	if err != nil {
		return models.Order{}, fmt.Errorf("query order %s: %w", orderID, err)
	}
	return o, nil
}

// CancelForUser cancels an order owned by userID and returns the durable stock
// to the product, both inside one transaction.
//
// The status change and the stock restoration must agree, and the cancellation
// must happen exactly once: two concurrent cancellations of the same order
// would otherwise each return the stock. The UPDATE therefore matches only
// orders that are still holding stock, and the row it locks serialises the
// racing requests, so exactly one of them observes a changed row.
//
// It returns the cancelled order so the caller knows how much stock to return
// to the in-memory counter.
func (r *OrderRepository) CancelForUser(ctx context.Context, orderID, userID uuid.UUID) (models.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Order{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const cancel = `
		UPDATE orders
		SET status = $1
		WHERE id = $2 AND user_id = $3 AND status = ANY($4)
		RETURNING id, user_id, product_id, qty, status, created_at`

	cancellable := pqStatuses(models.OrderStatusPending, models.OrderStatusSuccess)

	var o models.Order
	err = tx.QueryRowContext(ctx, cancel, models.OrderStatusCancelled, orderID, userID, cancellable).
		Scan(&o.ID, &o.UserID, &o.ProductID, &o.Qty, &o.Status, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Either the order is not this user's, or it is in a state that holds no
		// stock. Distinguish the two without revealing other users' orders.
		return models.Order{}, r.explainFailedCancel(ctx, orderID, userID)
	}
	if err != nil {
		return models.Order{}, fmt.Errorf("cancel order %s: %w", orderID, err)
	}

	const restore = `UPDATE products SET stock = stock + $1 WHERE id = $2`
	if _, err := tx.ExecContext(ctx, restore, o.Qty, o.ProductID); err != nil {
		return models.Order{}, fmt.Errorf("restore stock for product %s: %w", o.ProductID, err)
	}

	if err := tx.Commit(); err != nil {
		return models.Order{}, fmt.Errorf("commit transaction: %w", err)
	}
	return o, nil
}

// explainFailedCancel decides which error a no-op cancellation should report.
func (r *OrderRepository) explainFailedCancel(ctx context.Context, orderID, userID uuid.UUID) error {
	if _, err := r.FindByIDForUser(ctx, orderID, userID); err != nil {
		return err // ErrOrderNotFound, or a genuine query failure
	}
	return ErrOrderNotCancellable
}

// pqStatuses renders statuses as a PostgreSQL text array literal for use with
// ANY(), which keeps the status list a bound parameter rather than string
// concatenation.
func pqStatuses(statuses ...models.OrderStatus) string {
	out := make([]byte, 0, len(statuses)*12)
	out = append(out, '{')
	for i, s := range statuses {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, s...)
	}
	out = append(out, '}')
	return string(out)
}
