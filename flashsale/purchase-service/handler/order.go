package handler

import (
	"errors"
	"net/http"

	"github.com/flashsale/common/response"
	"github.com/flashsale/purchase-service/redis"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrderHandler exposes orders that are accepted but not yet stored.
type OrderHandler struct {
	store *redis.Client
}

// NewOrderHandler creates an OrderHandler.
func NewOrderHandler(store *redis.Client) *OrderHandler {
	return &OrderHandler{store: store}
}

// GetPending handles GET /internal/orders/:id/pending. The API gateway calls it
// when an order is not in PostgreSQL yet, so a buyer reading their order
// straight after purchasing sees it as PENDING rather than missing.
func (h *OrderHandler) GetPending(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", "order id must be a UUID")
		return
	}

	event, err := h.store.PendingOrder(c.Request.Context(), orderID)
	if errors.Is(err, redis.ErrNotPending) {
		response.Error(c, http.StatusNotFound, "Order not pending", "No pending order with this id")
		return
	}
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Failed to read order",
			"Order storage is unavailable")
		return
	}
	response.Success(c, http.StatusOK, "Pending order retrieved", event)
}
