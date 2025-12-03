package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OrderStatus represents the status of an order
type OrderStatus string

const (
	OrderStatusPending OrderStatus = "PENDING"
	OrderStatusSuccess OrderStatus = "SUCCESS"
	OrderStatusFailed  OrderStatus = "FAILED"
)

// Order represents an order in the database
type Order struct {
	ID        uuid.UUID
	UserID    int
	ProductID int
	Qty       int
	Status    OrderStatus
	CreatedAt time.Time
}

// OrderRepository handles order database operations
type OrderRepository struct {
	db *sql.DB
}

// NewOrderRepository creates a new OrderRepository
func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// Create inserts a new order into the database
func (r *OrderRepository) Create(order *Order) error {
	query := `
		INSERT INTO orders (id, user_id, product_id, qty, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := r.db.Exec(
		query,
		order.ID,
		order.UserID,
		order.ProductID,
		order.Qty,
		order.Status,
		order.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	return nil
}

// UpdateStatus updates the status of an order
func (r *OrderRepository) UpdateStatus(orderID uuid.UUID, status OrderStatus) error {
	query := `UPDATE orders SET status = $1 WHERE id = $2`

	result, err := r.db.Exec(query, status, orderID)
	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("order not found: %s", orderID)
	}

	return nil
}

// GetByID retrieves an order by ID
func (r *OrderRepository) GetByID(orderID uuid.UUID) (*Order, error) {
	query := `
		SELECT id, user_id, product_id, qty, status, created_at
		FROM orders
		WHERE id = $1
	`

	var order Order
	err := r.db.QueryRow(query, orderID).Scan(
		&order.ID,
		&order.UserID,
		&order.ProductID,
		&order.Qty,
		&order.Status,
		&order.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return &order, nil
}

// GetByUserID retrieves all orders for a user
func (r *OrderRepository) GetByUserID(userID int) ([]*Order, error) {
	query := `
		SELECT id, user_id, product_id, qty, status, created_at
		FROM orders
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	defer rows.Close()

	var orders []*Order
	for rows.Next() {
		var order Order
		err := rows.Scan(
			&order.ID,
			&order.UserID,
			&order.ProductID,
			&order.Qty,
			&order.Status,
			&order.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, &order)
	}

	return orders, nil
}
