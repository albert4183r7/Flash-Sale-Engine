package dto

import "github.com/google/uuid"

// PurchaseRequest represents a purchase request
type PurchaseRequest struct {
	ProductID uuid.UUID `json:"product_id" binding:"required,gt=0"`
	Qty       int 		`json:"qty" binding:"required,gt=0,lte=10"`
}

// PurchaseResponse represents a purchase response
type PurchaseResponse struct {
	OrderID   uuid.UUID 	`json:"order_id"`
	Status    string 		`json:"status"`
	Message   string 		`json:"message"`
	ProductID uuid.UUID    	`json:"product_id"`
	Qty       int    		`json:"qty"`
}

// OrderStatusResponse represents an order status response
type OrderStatusResponse struct {
	OrderID   uuid.UUID `json:"order_id"`
	UserID    uuid.UUID `json:"user_id"`
	ProductID uuid.UUID `json:"product_id"`
	Qty       int    	`json:"qty"`
	Status    string 	`json:"status"`
	CreatedAt string 	`json:"created_at"`
}
