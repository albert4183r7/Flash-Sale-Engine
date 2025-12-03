package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/flashsale/purchase-service/redis"
	"github.com/gin-gonic/gin"
)

// StockHandler handles stock-related requests
type StockHandler struct {
	redisClient *redis.Client
}

// NewStockHandler creates a new StockHandler
func NewStockHandler(redisClient *redis.Client) *StockHandler {
	return &StockHandler{redisClient: redisClient}
}

// GetStock returns the current stock for a product
func (h *StockHandler) GetStock(c *gin.Context) {
	productIDStr := c.Param("id")
	productID, err := strconv.Atoi(productIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid product ID",
			"error":   "Product ID must be a number",
		})
		return
	}

	stock, err := h.redisClient.GetStock(context.Background(), productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get stock",
			"error":   err.Error(),
		})
		return
	}

	if stock == -1 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Product not found",
			"error":   "Stock not initialized for this product",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Stock retrieved successfully",
		"data": gin.H{
			"product_id": productID,
			"stock":      stock,
		},
	})
}
