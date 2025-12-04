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
			"message": "Invalid request",
			"error":   err.Error(),
		})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "User not authenticated",
			"error":   "User ID not found in token",
		})
		return
	}

	resp, err := h.purchaseClient.Purchase(userID.(uuid.UUID), req.ProductID, req.Qty)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "Purchase service unavailable",
			"error":   err.Error(),
		})
		return
	}

	if !resp.Success {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": resp.Message,
			"error":   resp.Error,
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": resp.Message,
		"data": dto.PurchaseResponse{
			OrderID:   resp.OrderID,
			Status:    "PENDING",
			Message:   "Your order is being processed",
			ProductID: req.ProductID,
			Qty:       req.Qty,
		},
	})
}
