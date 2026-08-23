package handler

import (
	"net/http"

	"github.com/flashsale/api-gateway/client"
	"github.com/flashsale/api-gateway/dto"
	"github.com/flashsale/api-gateway/middleware"
	"github.com/flashsale/common/models"
	"github.com/flashsale/common/response"
	"github.com/gin-gonic/gin"
)

// PurchaseHandler forwards purchases to the purchase service.
type PurchaseHandler struct {
	purchases *client.Purchase
}

// NewPurchaseHandler creates a PurchaseHandler.
func NewPurchaseHandler(purchases *client.Purchase) *PurchaseHandler {
	return &PurchaseHandler{purchases: purchases}
}

// Purchase handles POST /purchase.
func (h *PurchaseHandler) Purchase(c *gin.Context) {
	var req dto.PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	// The buyer comes from the verified token, so a client cannot order on
	// somebody else's behalf by putting their ID in the body.
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "User not authenticated",
			"No authenticated user on this request")
		return
	}

	result, err := h.purchases.Create(c.Request.Context(), userID, req.ProductID, req.Qty)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Purchase service unavailable",
			"The purchase service could not be reached")
		return
	}

	if !result.Success {
		// Pass the purchase service's own status through, so a sold-out product
		// stays a 409 and an unknown product stays a 404 instead of every
		// refusal collapsing into one code.
		response.Error(c, result.Status, result.Message, result.Error)
		return
	}

	response.Success(c, http.StatusAccepted, "Purchase accepted and queued for processing",
		dto.PurchaseResponse{
			OrderID:   result.OrderID,
			Status:    models.OrderStatusPending,
			ProductID: req.ProductID,
			Qty:       req.Qty,
		})
}
