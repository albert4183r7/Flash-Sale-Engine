package handler

import (
	"errors"
	"net/http"

	"github.com/flashsale/common/response"
	"github.com/flashsale/purchase-service/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PurchaseRequest is the body the API gateway sends. The user ID is supplied by
// the gateway after it has authenticated the caller.
//
// The lte bound caps how much stock a single request can remove during a sale;
// struct tags cannot reference a constant, so the limit lives in the tag.
type PurchaseRequest struct {
	UserID    uuid.UUID `json:"user_id" binding:"required"`
	ProductID uuid.UUID `json:"product_id" binding:"required"`
	Qty       int       `json:"qty" binding:"required,gt=0,lte=10"`
}

// PurchaseHandler serves purchase requests.
type PurchaseHandler struct {
	purchases *service.Purchase
}

// NewPurchaseHandler creates a PurchaseHandler.
func NewPurchaseHandler(purchases *service.Purchase) *PurchaseHandler {
	return &PurchaseHandler{purchases: purchases}
}

// Purchase handles POST /purchase.
func (h *PurchaseHandler) Purchase(c *gin.Context) {
	var req PurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	orderID, err := h.purchases.Process(c.Request.Context(), req.UserID, req.ProductID, req.Qty)
	if err != nil {
		status, message := purchaseFailureStatus(err)
		response.Error(c, status, message, err.Error())
		return
	}

	response.Success(c, http.StatusAccepted, "Purchase accepted and queued for processing", gin.H{
		"order_id": orderID,
	})
}

// purchaseFailureStatus maps a purchase failure to the HTTP status that
// describes it, so clients can tell a sold-out product from a bad request or an
// outage.
func purchaseFailureStatus(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrDuplicatePurchase):
		return http.StatusConflict, "Duplicate purchase request"
	case errors.Is(err, service.ErrOutOfStock):
		return http.StatusConflict, "Out of stock"
	case errors.Is(err, service.ErrProductUnknown):
		return http.StatusNotFound, "Product not found"
	default:
		return http.StatusServiceUnavailable, "Purchase could not be processed"
	}
}
