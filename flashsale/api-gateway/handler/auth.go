package handler

import (
	"net/http"

	"github.com/flashsale/api-gateway/client"
	"github.com/flashsale/api-gateway/dto"
	"github.com/gin-gonic/gin"
)

// AuthHandler proxies authentication requests to user-service
type AuthHandler struct {
	userClient *client.UserClient
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(userClient *client.UserClient) *AuthHandler {
	return &AuthHandler{userClient: userClient}
}

// Signup handles user registration
func (h *AuthHandler) Signup(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "Please provide a valid email and password.",
			"error":   "INVALID_REQUEST",
		})
		return
	}

	result, statusCode, err := h.userClient.Signup(req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Our registration system is temporarily unavailable. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}

	if result.Error != "" {
		c.JSON(statusCode, gin.H{
			"success": false,
			"data":    nil,
			"message": result.Error,
			"error":   "SIGNUP_FAILED",
		})
		return
	}

	c.JSON(statusCode, gin.H{
		"success": true,
		"data": gin.H{
			"user_id": result.UserID,
		},
		"message": "Account created successfully. You can now log in.",
	})
}

// Login handles user authentication
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "Please provide a valid email and password.",
			"error":   "INVALID_REQUEST",
		})
		return
	}

	result, statusCode, err := h.userClient.Login(req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    nil,
			"message": "Our login system is temporarily unavailable. Please try again.",
			"error":   "SERVICE_UNAVAILABLE",
		})
		return
	}

	if result.Error != "" {
		c.JSON(statusCode, gin.H{
			"success": false,
			"data":    nil,
			"message": "Invalid email or password. Please try again.",
			"error":   "INVALID_CREDENTIALS",
		})
		return
	}

	c.JSON(statusCode, gin.H{
		"success": true,
		"data": dto.LoginResponse{
			Token:     result.Data.Token,
			ExpiresIn: result.Data.ExpiresIn,
			UserID:    result.Data.UserID,
			Email:     result.Data.Email,
			Role:      result.Data.Role,
		},
		"message": "Login successful. Welcome back!",
	})
}