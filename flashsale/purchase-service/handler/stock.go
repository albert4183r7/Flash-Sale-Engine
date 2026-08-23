package handler

import (
	"errors"
	"net/http"

	"github.com/flashsale/common/response"
	"github.com/flashsale/purchase-service/redis"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StockHandler exposes the in-memory stock counters.
type StockHandler struct {
	stock *redis.Client
}

// NewStockHandler creates a StockHandler.
func NewStockHandler(stock *redis.Client) *StockHandler {
	return &StockHandler{stock: stock}
}

// GetStock handles GET /stock/:id.
func (h *StockHandler) GetStock(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", "product id must be a UUID")
		return
	}

	remaining, err := h.stock.GetStock(c.Request.Context(), productID)
	if errors.Is(err, redis.ErrProductUnknown) {
		response.Error(c, http.StatusNotFound, "Product not found",
			"No stock counter exists for this product")
		return
	}
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Failed to read stock",
			"Stock storage is unavailable")
		return
	}

	response.Success(c, http.StatusOK, "Stock retrieved", gin.H{
		"product_id": productID,
		"stock":      remaining,
	})
}

// RestoreStockRequest returns previously reserved units to the counter.
type RestoreStockRequest struct {
	ProductID uuid.UUID `json:"product_id" binding:"required"`
	Qty       int       `json:"qty" binding:"required,gt=0"`
}

// RestoreStock handles POST /internal/stock/restore, called by the API gateway
// when an order is cancelled.
func (h *StockHandler) RestoreStock(c *gin.Context) {
	var req RestoreStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	if err := h.stock.IncrementStock(c.Request.Context(), req.ProductID, req.Qty); err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Failed to restore stock",
			"Stock storage is unavailable")
		return
	}

	response.Success(c, http.StatusOK, "Stock restored", nil)
}
