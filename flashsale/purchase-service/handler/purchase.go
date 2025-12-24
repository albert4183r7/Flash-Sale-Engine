package handler

import (
	"net/http"

	"github.com/flashsale/purchase-service/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PurchaseRequest represents an incoming purchase request
type PurchaseRequest struct {
	UserID        uuid.UUID `json:"user_id" binding:"required,gt=0"`
	ProductID     uuid.UUID `json:"product_id" binding:"required,gt=0"`
	Qty           int       `json:"qty" binding:"required,gt=0,lte=10"`
	Notes         string    `json:"notes"`
	PaymentMethod string    `json:"payment_method"`
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
			"data":    nil,
			"message": "Please provide valid purchase details.",
			"error":   "INVALID_REQUEST",
		})
		return
	}

	result := h.purchaseService.ProcessPurchase(c.Request.Context(), req.UserID, req.ProductID, req.Qty, req.Notes, req.PaymentMethod)

	if !result.Success {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data": gin.H{
				"product_id": result.ProductID,
			},
			"message": result.Message,
			"error":   result.ErrorCode,
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"data": gin.H{
			"order_id":     result.OrderID,
			"product_id":   result.ProductID,
			"product_name": result.ProductName,
			"qty":          result.Qty,
			"status":       "PENDING",
		},
		"message": result.Message,
	})
}
