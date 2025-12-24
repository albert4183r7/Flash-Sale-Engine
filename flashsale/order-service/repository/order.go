package repository

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"
	OrderStatusSuccess   OrderStatus = "SUCCESS"
	OrderStatusFailed    OrderStatus = "FAILED"
	OrderStatusCancelled OrderStatus = "CANCELLED"
)

type Order struct {
	ID           uuid.UUID   `json:"id"`
	UserID       uuid.UUID   `json:"user_id"`
	ProductID    uuid.UUID   `json:"product_id"`
	ProductName  string      `json:"product_name"`
	ProductPrice int         `json:"product_price"`
	Qty          int         `json:"qty"`
	Notes        string      `json:"notes,omitempty"`
	Status       OrderStatus `json:"status"`
	CreatedAt    time.Time   `json:"created_at"`
}

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(order *Order) error {
	_, err := r.db.Exec(
		`INSERT INTO orders (id, user_id, product_id, product_name, product_price, qty, notes, status, created_at) 
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		order.ID, order.UserID, order.ProductID, order.ProductName, order.ProductPrice, order.Qty, order.Notes, order.Status, order.CreatedAt,
	)
	return err
}

func (r *OrderRepository) FindByID(id uuid.UUID) (*Order, error) {
	var order Order
	var notes sql.NullString
	err := r.db.QueryRow(
		`SELECT id, user_id, product_id, product_name, product_price, qty, notes, status, created_at FROM orders WHERE id = $1`,
		id,
	).Scan(&order.ID, &order.UserID, &order.ProductID, &order.ProductName, &order.ProductPrice, &order.Qty, &notes, &order.Status, &order.CreatedAt)
	if err != nil {
		return nil, err
	}
	order.Notes = notes.String
	return &order, nil
}

func (r *OrderRepository) FindByUserID(userID uuid.UUID) ([]Order, error) {
	rows, err := r.db.Query(
		`SELECT id, user_id, product_id, product_name, product_price, qty, notes, status, created_at FROM orders WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		var notes sql.NullString
		if err := rows.Scan(&o.ID, &o.UserID, &o.ProductID, &o.ProductName, &o.ProductPrice, &o.Qty, &notes, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.Notes = notes.String
		orders = append(orders, o)
	}
	return orders, nil
}

func (r *OrderRepository) UpdateStatus(id uuid.UUID, status OrderStatus) error {
	result, err := r.db.Exec(
		`UPDATE orders SET status = $1 WHERE id = $2`,
		status, id,
	)
	if err != nil {
		return err
	}
	
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *OrderRepository) Cancel(id uuid.UUID) (*Order, error) {
	// First get order details for stock restoration
	order, err := r.FindByID(id)
	if err != nil {
		return nil, err
	}
	
	if order.Status == OrderStatusCancelled {
		return order, nil // Already cancelled
	}

	// Update status
	err = r.UpdateStatus(id, OrderStatusCancelled)
	if err != nil {
		return nil, err
	}
	
	order.Status = OrderStatusCancelled
	return order, nil
}
