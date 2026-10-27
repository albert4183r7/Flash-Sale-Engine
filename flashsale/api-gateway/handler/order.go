package handler

import (
	"errors"
	"net/http"

	"github.com/flashsale/api-gateway/dto"
	"github.com/flashsale/api-gateway/middleware"
	"github.com/flashsale/api-gateway/service"
	"github.com/flashsale/common/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrderHandler serves order lookups and cancellations.
type OrderHandler struct {
	orders *service.Order
}

// NewOrderHandler creates an OrderHandler.
func NewOrderHandler(orders *service.Order) *OrderHandler {
	return &OrderHandler{orders: orders}
}

// GetOrder handles GET /orders/:id.
func (h *OrderHandler) GetOrder(c *gin.Context) {
	userID, orderID, ok := h.identify(c)
	if !ok {
		return
	}

	order, err := h.orders.Get(c.Request.Context(), orderID, userID)
	switch {
	case err == nil:
		response.Success(c, http.StatusOK, "Order retrieved", dto.NewOrderResponse(order))
	case errors.Is(err, service.ErrOrderNotFound):
		response.Error(c, http.StatusNotFound, "Order not found",
			"No such order exists for this account")
	default:
		response.Error(c, http.StatusInternalServerError, "Failed to load order",
			"Unable to read the order")
	}
}

// CancelOrder handles DELETE /orders/:id.
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	userID, orderID, ok := h.identify(c)
	if !ok {
		return
	}

	order, err := h.orders.Cancel(c.Request.Context(), orderID, userID)
	switch {
	case err == nil:
		response.Success(c, http.StatusOK, "Order cancelled and stock restored",
			dto.NewOrderResponse(order))
	case errors.Is(err, service.ErrOrderNotFound):
		response.Error(c, http.StatusNotFound, "Order not found",
			"No such order exists for this account")
	case errors.Is(err, service.ErrOrderNotCancellable):
		response.Error(c, http.StatusConflict, "Order cannot be cancelled",
			"This order has already been cancelled")
	default:
		response.Error(c, http.StatusInternalServerError, "Failed to cancel order",
			"Unable to cancel the order")
	}
}

// identify resolves the authenticated buyer and the order they addressed,
// writing the error response itself when either is unusable.
func (h *OrderHandler) identify(c *gin.Context) (userID, orderID uuid.UUID, ok bool) {
	userID, ok = middleware.UserID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "User not authenticated",
			"No authenticated user on this request")
		return uuid.Nil, uuid.Nil, false
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", "order id must be a UUID")
		return uuid.Nil, uuid.Nil, false
	}
	return userID, orderID, true
}
