package handler

import (
	"context"
	"net/http"

	"github.com/flashsale/purchase-service/redis"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type StockHandler struct {
	redisClient *redis.Client
}

func NewStockHandler(redisClient *redis.Client) *StockHandler {
	return &StockHandler{redisClient: redisClient}
}

func (h *StockHandler) GetStock(c *gin.Context) {
	productIDStr := c.Param("id")
	
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Product UUID"})
		return
	}

	stock, _ := h.redisClient.GetStock(context.Background(), productID)
	c.JSON(http.StatusOK, gin.H{"product_id": productID, "stock": stock})
}

// RestoreStock handles atomic increment for cancellations
func (h *StockHandler) RestoreStock(c *gin.Context) {
	var req struct {
		ProductID uuid.UUID `json:"product_id"`
		Qty       int 		`json:"qty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.redisClient.IncrementStock(context.Background(), req.ProductID, req.Qty)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Redis error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}