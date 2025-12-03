package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrderHandler handles order-related requests
// NOTE: In a full implementation, this would query the Order Worker's database
// via a dedicated Order Query Service. For this portfolio project, we demonstrate
// the API contract while keeping the architecture simple.
type OrderHandler struct{}

// NewOrderHandler creates a new OrderHandler
func NewOrderHandler() *OrderHandler {
	return &OrderHandler{}
}

// GetOrder returns the status of an order
// In production, this endpoint would:
// 1. Query PostgreSQL via Order Query Service
// 2. Return real order status (PENDING, SUCCESS, FAILED)
// 3. Include caching for frequently accessed orders
func (h *OrderHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")

	if _, err := uuid.Parse(orderID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid order ID",
			"error":   "Order ID must be a valid UUID",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Order status retrieved",
		"data": gin.H{
			"order_id":    orderID,
			"status":      "PENDING",
			"description": "Order is being processed asynchronously by Order Worker",
		},
	})
}
