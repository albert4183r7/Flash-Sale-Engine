package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthHandler handles health check requests
type HealthHandler struct{}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health returns the health status of the Purchase Service
func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Purchase Service is healthy",
		"data": gin.H{
			"service": "purchase-service",
			"status":  "healthy",
			"version": "1.0.0",
		},
	})
}
