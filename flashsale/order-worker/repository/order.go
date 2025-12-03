package repository

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderStatusPending OrderStatus = "PENDING"
	OrderStatusSuccess OrderStatus = "SUCCESS"
	OrderStatusFailed  OrderStatus = "FAILED"
)

type Order struct {
	ID        uuid.UUID
	UserID    int
	ProductID int
	Qty       int
	Status    OrderStatus
	CreatedAt time.Time
}

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(order *Order) error {
	query := `INSERT INTO orders (id, user_id, product_id, qty, status, created_at) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.Exec(query, order.ID, order.UserID, order.ProductID, order.Qty, order.Status, order.CreatedAt)
	return err
}

func (r *OrderRepository) UpdateStatus(orderID uuid.UUID, status OrderStatus) error {
	_, err := r.db.Exec(`UPDATE orders SET status = $1 WHERE id = $2`, status, orderID)
	return err
}

// DecreaseProductStock syncs the Redis decrement to the Postgres DB
func (r *OrderRepository) DecreaseProductStock(productID, qty int) error {
	_, err := r.db.Exec(`UPDATE products SET stock = stock - $1 WHERE id = $2`, qty, productID)
	return err
}