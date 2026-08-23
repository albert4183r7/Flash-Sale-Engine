package handler

import (
	"net/http"

	"github.com/flashsale/common/response"
	"github.com/gin-gonic/gin"
)

// HealthHandler answers health checks.
type HealthHandler struct {
	checkDB func(c *gin.Context) error
}

// NewHealthHandler creates a HealthHandler. checkDB is called on each probe, so
// a gateway that cannot reach PostgreSQL reports itself unhealthy rather than
// accepting traffic it cannot serve.
func NewHealthHandler(checkDB func(c *gin.Context) error) *HealthHandler {
	return &HealthHandler{checkDB: checkDB}
}

// Health handles GET /health.
func (h *HealthHandler) Health(c *gin.Context) {
	if err := h.checkDB(c); err != nil {
		response.Error(c, http.StatusServiceUnavailable, "API Gateway is unhealthy",
			"PostgreSQL is unreachable")
		return
	}

	response.Success(c, http.StatusOK, "API Gateway is healthy", gin.H{
		"service": "api-gateway",
		"status":  "healthy",
	})
}
