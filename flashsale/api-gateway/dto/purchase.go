package dto

import (
	"time"

	"github.com/flashsale/common/models"
	"github.com/google/uuid"
)

// PurchaseRequest is the body of POST /purchase. The buyer is taken from the
// access token, never from the request body.
type PurchaseRequest struct {
	ProductID uuid.UUID `json:"product_id" binding:"required"`
	Qty       int       `json:"qty" binding:"required,gt=0,lte=10"`
}

// PurchaseResponse acknowledges an accepted purchase. The order is not yet
// persisted at this point, which is why the status is PENDING.
type PurchaseResponse struct {
	OrderID   uuid.UUID          `json:"order_id"`
	Status    models.OrderStatus `json:"status"`
	ProductID uuid.UUID          `json:"product_id"`
	Qty       int                `json:"qty"`
}

// OrderResponse describes a stored order.
type OrderResponse struct {
	OrderID   uuid.UUID          `json:"order_id"`
	UserID    uuid.UUID          `json:"user_id"`
	ProductID uuid.UUID          `json:"product_id"`
	Qty       int                `json:"qty"`
	Status    models.OrderStatus `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
}

// NewOrderResponse converts a stored order into its API representation.
func NewOrderResponse(o models.Order) OrderResponse {
	return OrderResponse{
		OrderID:   o.ID,
		UserID:    o.UserID,
		ProductID: o.ProductID,
		Qty:       o.Qty,
		Status:    o.Status,
		CreatedAt: o.CreatedAt,
	}
}
