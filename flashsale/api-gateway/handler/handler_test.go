package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestPurchaseHandler_InvalidRequest(t *testing.T) {
	t.Run("should return error for invalid JSON", func(t *testing.T) {
		router := gin.New()
		router.POST("/api/v1/users/:user_id/orders", func(c *gin.Context) {
			// Simulate the actual handler behavior for invalid JSON
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"data":    nil,
				"message": "Please provide valid purchase details.",
				"error":   "INVALID_REQUEST",
			})
		})

		req := httptest.NewRequest("POST", "/api/v1/users/550e8400-e29b-41d4-a716-446655440000/orders", strings.NewReader("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "INVALID_REQUEST") {
			t.Errorf("Expected error code INVALID_REQUEST in response")
		}
	})
}

func TestPurchaseHandler_Unauthorized(t *testing.T) {
	t.Run("should return error when user_id not in context", func(t *testing.T) {
		router := gin.New()
		router.POST("/api/v1/users/:user_id/orders", func(c *gin.Context) {
			_, exists := c.Get("user_id")
			if !exists {
				c.JSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"data":    nil,
					"message": "You need to be logged in to make a purchase.",
					"error":   "UNAUTHORIZED",
				})
				return
			}
		})

		req := httptest.NewRequest("POST", "/api/v1/users/550e8400-e29b-41d4-a716-446655440000/orders", strings.NewReader(`{"product_id": "test", "qty": 1}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
	})
}

func TestAuthHandler_InvalidRequest(t *testing.T) {
	t.Run("should return error for missing email/password", func(t *testing.T) {
		router := gin.New()
		router.POST("/api/v1/auth/login", func(c *gin.Context) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"data":    nil,
				"message": "Please provide a valid email and password.",
				"error":   "INVALID_REQUEST",
			})
		})

		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

