package handler

import (
	"database/sql"
	"net/http"

	"github.com/flashsale/api-gateway/client"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type OrderHandler struct {
	db             *sql.DB
	purchaseClient *client.PurchaseClient
}

func NewOrderHandler(db *sql.DB, purchaseClient *client.PurchaseClient) *OrderHandler {
	return &OrderHandler{db: db, purchaseClient: purchaseClient}
}

func (h *OrderHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")
	var status string
	err := h.db.QueryRow("SELECT status FROM orders WHERE id = $1", orderID).Scan(&status)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"order_id": orderID, "status": status})
}

// CancelOrder handles Soft Delete + Stock Restore
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID"})
		return
	}

	// 1. Get Order Details (need product_id and qty)
	var productID, qty int
	var status string
	err = h.db.QueryRow("SELECT product_id, qty, status FROM orders WHERE id = $1", orderID).Scan(&productID, &qty, &status)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}

	if status == "CANCELLED" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Order already cancelled"})
		return
	}

	// 2. Call Purchase Service to Restore Stock in Redis
	// Note: In a robust system, this should be an idempotent event or transaction
	err = h.purchaseClient.RestoreStock(productID, qty)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to restore stock"})
		return
	}

	// 3. Soft Delete in DB
	_, err = h.db.Exec("UPDATE orders SET status = 'CANCELLED' WHERE id = $1", orderID)
	if err != nil {
		// Critical: Stock was restored but DB failed. In prod, we need retry logic here.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update order status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Order cancelled and stock restored"})
}