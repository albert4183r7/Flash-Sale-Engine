package handler

import (
	"net/http"

	"github.com/flashsale/api-gateway/client"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ProductHandler proxies product requests to product-service
type ProductHandler struct {
	productClient *client.ProductClient
}

func NewProductHandler(productClient *client.ProductClient) *ProductHandler {
	return &ProductHandler{productClient: productClient}
}

func (h *ProductHandler) ListProducts(c *gin.Context) {
	products, err := h.productClient.ListProducts()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Product service unavailable"})
		return
	}
	c.JSON(http.StatusOK, products)
}

func (h *ProductHandler) GetProduct(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID"})
		return
	}

	product, err := h.productClient.GetProduct(id)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Product service unavailable"})
		return
	}
	c.JSON(http.StatusOK, product)
}

func (h *ProductHandler) GetStock(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID"})
		return
	}

	stock, err := h.productClient.GetStock(id)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Product service unavailable"})
		return
	}
	c.JSON(http.StatusOK, stock)
}
