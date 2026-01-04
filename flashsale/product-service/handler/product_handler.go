package handler

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/flashsale/product-service/cache"
	"github.com/flashsale/product-service/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ProductHandler struct {
	productRepo *repository.ProductRepository
	stockCache  *cache.StockCache
}

func NewProductHandler(productRepo *repository.ProductRepository, stockCache *cache.StockCache) *ProductHandler {
	return &ProductHandler{
		productRepo: productRepo,
		stockCache:  stockCache,
	}
}

// ListProducts returns all products
func (h *ProductHandler) ListProducts(c *gin.Context) {
	products, err := h.productRepo.FindAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"products": products})
}

// GetProduct returns a single product by ID
func (h *ProductHandler) GetProduct(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID"})
		return
	}

	product, err := h.productRepo.FindByID(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, product)
}

// GetStock returns current stock from Redis cache
func (h *ProductHandler) GetStock(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID"})
		return
	}

	stock, err := h.stockCache.GetStock(context.Background(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cache error"})
		return
	}

	if stock == -1 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found in cache"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"product_id": id,
		"stock":      stock,
	})
}

type StockRequest struct {
	ProductID uuid.UUID `json:"product_id" binding:"required"`
	Qty       int       `json:"qty" binding:"required,min=1"`
}

// DecrementStock atomically decreases stock (called by purchase-service)
func (h *ProductHandler) DecrementStock(c *gin.Context) {
	var req StockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newStock, err := h.stockCache.DecrementStock(context.Background(), req.ProductID, req.Qty)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decrement stock"})
		return
	}

	if newStock == -2 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Product not found",
		})
		return
	}

	if newStock < 0 {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "Insufficient stock",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"new_stock": newStock,
	})
}

// RestoreStock increments stock (called for order cancellations)
func (h *ProductHandler) RestoreStock(c *gin.Context) {
	var req StockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Restore in Redis
	newStock, err := h.stockCache.IncrementStock(context.Background(), req.ProductID, req.Qty)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore stock in cache"})
		return
	}

	// Also restore in DB for consistency
	if err := h.productRepo.UpdateStock(req.ProductID, req.Qty); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore stock in database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"new_stock": newStock,
	})
}

// SyncStockToDB decreases stock in DB (called by order-worker for eventual consistency)
func (h *ProductHandler) SyncStockToDB(c *gin.Context) {
	var req StockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.productRepo.DecreaseStock(req.ProductID, req.Qty); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync stock to database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// WarmupCache loads all product stock from DB to Redis
func (h *ProductHandler) WarmupCache(c *gin.Context) {
	products, err := h.productRepo.FindAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}

	ctx := context.Background()
	count := 0
	for _, p := range products {
		if err := h.stockCache.InitializeStock(ctx, p.ID, p.Stock); err == nil {
			count++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cache warmed up",
		"count":   count,
	})
}
