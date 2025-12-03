package handler

import (
	"net/http"
	"time"

	"github.com/flashsale/api-gateway/dto"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// User represents a user for authentication
type User struct {
	ID       int
	Email    string
	Password string
	Role     string
}

// DummyUsers for authentication (in production, use a database)
var DummyUsers = map[string]User{
	"user@example.com": {
		ID:       1,
		Email:    "user@example.com",
		Password: "password123",
		Role:     "user",
	},
	"admin@example.com": {
		ID:       2,
		Email:    "admin@example.com",
		Password: "admin123",
		Role:     "admin",
	},
	"buyer@example.com": {
		ID:       3,
		Email:    "buyer@example.com",
		Password: "buyer123",
		Role:     "user",
	},
}

// AuthHandler handles authentication requests
type AuthHandler struct {
	jwtSecret string
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(jwtSecret string) *AuthHandler {
	return &AuthHandler{jwtSecret: jwtSecret}
}

// Login handles user login and returns JWT token
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
			"error":   err.Error(),
		})
		return
	}

	user, exists := DummyUsers[req.Email]
	if !exists || user.Password != req.Password {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication failed",
			"error":   "Invalid email or password",
		})
		return
	}

	expiresIn := time.Hour * 24
	expirationTime := time.Now().Add(expiresIn)

	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"exp":     expirationTime.Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to generate token",
			"error":   "Internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Login successful",
		"data": dto.LoginResponse{
			Token:     tokenString,
			ExpiresIn: int64(expiresIn.Seconds()),
			UserID:    user.ID,
			Email:     user.Email,
			Role:      user.Role,
		},
	})
}
