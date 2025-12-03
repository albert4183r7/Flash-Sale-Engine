package models

import (
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

// Order represents an order in the system
type Order struct {
	ID        uuid.UUID   `json:"id"`
	UserID    int         `json:"user_id"`
	ProductID int         `json:"product_id"`
	Qty       int         `json:"qty"`
	Status    OrderStatus `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}

// OrderEvent is the message published to RabbitMQ
type OrderEvent struct {
	OrderID   uuid.UUID `json:"order_id"`
	UserID    int       `json:"user_id"`
	ProductID int       `json:"product_id"`
	Qty       int       `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}
