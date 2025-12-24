package handler

import (
	"net/http"

	"github.com/flashsale/api-gateway/client"
	"github.com/flashsale/api-gateway/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PurchaseHandler handles purchase requests
type PurchaseHandler struct {
	purchaseClient *client.PurchaseClient
}

// NewPurchaseHandler creates a new PurchaseHandler
func NewPurchaseHandler(purchaseClient *client.PurchaseClient) *PurchaseHandler {
	return &PurchaseHandler{purchaseClient: purchaseClient}
}

// Purchase handles a purchase request
func (h *PurchaseHandler) Purchase(c *gin.Context) {
	var req dto.PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "Please provide valid purchase details.",
			"error":   "INVALID_REQUEST",
		})
		return
	}

	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"data":    nil,
			"message": "You need to be logged in to make a purchase.",
			"error":   "UNAUTHORIZED",
		})
		return
	}

	// Parse user_id string to UUID
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "Invalid user session. Please log in again.",
			"error":   "INVALID_USER_ID",
		})
		return
	}

	resp, err := h.purchaseClient.Purchase(userID, req.ProductID, req.Qty, req.Notes, req.PaymentMethod)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Our purchase system is temporarily unavailable. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data": gin.H{
				"product_id": resp.Data.ProductID,
			},
			"message": resp.Message,
			"error":   resp.Error,
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"data": gin.H{
			"order_id":     resp.Data.OrderID,
			"product_id":   resp.Data.ProductID,
			"product_name": resp.Data.ProductName,
			"qty":          resp.Data.Qty,
			"status":       resp.Data.Status,
		},
		"message": resp.Message,
	})
}
