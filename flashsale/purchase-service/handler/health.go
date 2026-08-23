// Package handler contains the purchase service's HTTP handlers.
package handler

import (
	"net/http"

	"github.com/flashsale/common/response"
	"github.com/gin-gonic/gin"
)

// HealthHandler answers health checks.
type HealthHandler struct {
	checkRedis func(c *gin.Context) error
}

// NewHealthHandler creates a HealthHandler. checkRedis is called on each probe
// so an unreachable Redis is reported as unhealthy: the service cannot sell
// anything without it, and an orchestrator should not route traffic here.
func NewHealthHandler(checkRedis func(c *gin.Context) error) *HealthHandler {
	return &HealthHandler{checkRedis: checkRedis}
}

// Health handles GET /health.
func (h *HealthHandler) Health(c *gin.Context) {
	if err := h.checkRedis(c); err != nil {
		response.Error(c, http.StatusServiceUnavailable,
			"Purchase Service is unhealthy", "Redis is unreachable")
		return
	}

	response.Success(c, http.StatusOK, "Purchase Service is healthy", gin.H{
		"service": "purchase-service",
		"status":  "healthy",
	})
}
