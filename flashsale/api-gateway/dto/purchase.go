package dto

// PurchaseRequest represents a purchase request
type PurchaseRequest struct {
	ProductID int `json:"product_id" binding:"required,gt=0"`
	Qty       int `json:"qty" binding:"required,gt=0,lte=10"`
}

// PurchaseResponse represents a purchase response
type PurchaseResponse struct {
	OrderID   string `json:"order_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	ProductID int    `json:"product_id"`
	Qty       int    `json:"qty"`
}

// OrderStatusResponse represents an order status response
type OrderStatusResponse struct {
	OrderID   string `json:"order_id"`
	UserID    int    `json:"user_id"`
	ProductID int    `json:"product_id"`
	Qty       int    `json:"qty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}
