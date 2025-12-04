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

// CancelOrder handles Soft Delete + Stock Restore (Redis & Postgres)
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
	// Use QueryRowContext for better timeout control
	err = h.db.QueryRowContext(c, "SELECT product_id, qty, status FROM orders WHERE id = $1", orderID).Scan(&productID, &qty, &status)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}

	if status == "CANCELLED" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Order already cancelled"})
		return
	}

	// 2. Restore Stock in Redis (Cache Layer)
	// We do this *before* the DB transaction. If Redis fails, we stop here.
	// In a distributed system, this might lead to race conditions, but for flash sales,
	// keeping Redis accurate is priority #1 for availability.
	err = h.purchaseClient.RestoreStock(productID, qty)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to restore stock in cache"})
		return
	}

	// 3. Database Transaction (Persistence Layer)
	// We wrap the Status Update and Stock Restoration in a transaction for atomicity.
	tx, err := h.db.BeginTx(c, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start database transaction"})
		return
	}
	// Defer a rollback in case of panic or error, though we commit explicitly at the end.
	defer tx.Rollback()

	// 3a. Update Order Status
	res, err := tx.ExecContext(c, "UPDATE orders SET status = 'CANCELLED' WHERE id = $1", orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update order status"})
		return
	}
	
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Order was updated by another process"})
		return
	}

	// 3b. Restore Stock in PostgreSQL (The missing piece)
	_, err = tx.ExecContext(c, "UPDATE products SET stock = stock + $1 WHERE id = $2", qty, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore stock in database"})
		return
	}

	// 4. Commit Transaction
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Order cancelled and stock restored successfully"})
}