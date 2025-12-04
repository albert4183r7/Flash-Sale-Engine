package handler

import (
	"net/http"

	"github.com/flashsale/purchase-service/service"
	"github.com/gin-gonic/gin"
)

// PurchaseRequest represents an incoming purchase request
type PurchaseRequest struct {
	UserID    int `json:"user_id" binding:"required,gt=0"`
	ProductID int `json:"product_id" binding:"required,gt=0"`
	Qty       int `json:"qty" binding:"required,gt=1,lte=10"`
}

// PurchaseHandler handles purchase requests
type PurchaseHandler struct {
	purchaseService *service.PurchaseService
}

// NewPurchaseHandler creates a new PurchaseHandler
func NewPurchaseHandler(purchaseService *service.PurchaseService) *PurchaseHandler {
	return &PurchaseHandler{purchaseService: purchaseService}
}

// Purchase handles a purchase request
func (h *PurchaseHandler) Purchase(c *gin.Context) {
	var req PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
			"error":   err.Error(),
		})
		return
	}

	result := h.purchaseService.ProcessPurchase(c.Request.Context(), req.UserID, req.ProductID, req.Qty)

	if !result.Success {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": result.Message,
			"error":   result.Error,
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success":  true,
		"message":  result.Message,
		"order_id": result.OrderID.String(),
	})
}
