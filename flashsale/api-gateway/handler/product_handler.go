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
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Unable to retrieve products. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    products,
		"message": "Products retrieved successfully.",
	})
}

func (h *ProductHandler) GetProduct(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "The product ID provided is not valid.",
			"error":   "INVALID_PRODUCT_ID",
		})
		return
	}

	product, err := h.productClient.GetProduct(id)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Unable to retrieve product. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    product,
		"message": "Product retrieved successfully.",
	})
}

func (h *ProductHandler) GetStock(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "The product ID provided is not valid.",
			"error":   "INVALID_PRODUCT_ID",
		})
		return
	}

	stock, err := h.productClient.GetStock(id)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Unable to retrieve stock information. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stock,
		"message": "Stock retrieved successfully.",
	})
}

