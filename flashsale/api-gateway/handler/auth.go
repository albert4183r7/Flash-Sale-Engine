package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/flashsale/api-gateway/dto"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db          *sql.DB
	redisClient *redis.Client
	jwtSecret   string
}

// Update Constructor
func NewAuthHandler(db *sql.DB, redisClient *redis.Client, jwtSecret string) *AuthHandler {
	return &AuthHandler{db: db, redisClient: redisClient, jwtSecret: jwtSecret}
}

// Signup remains the same...
func (h *AuthHandler) Signup(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	_, err = h.db.Exec("INSERT INTO users (email, password_hash, role) VALUES ($1, $2, 'user')", req.Email, string(hashedPassword))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User likely already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
}

// Login with Caching
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Define a struct that matches what we want to cache
	type CachedUser struct {
		ID       uuid.UUID  `json:"id"`
		Email    string 	`json:"email"`
		Password string 	`json:"password"`
		Role     string 	`json:"role"`
	}
	var user CachedUser

	// 1. CACHE CHECK (Redis)
	cacheKey := fmt.Sprintf("user:%s", req.Email)
	val, err := h.redisClient.Get(context.Background(), cacheKey).Result()
	
	cacheHit := false
	if err == nil {
		// Cache Hit!
		if jsonErr := json.Unmarshal([]byte(val), &user); jsonErr == nil {
			cacheHit = true
		}
	}

	// 2. CACHE MISS (Database Query)
	if !cacheHit {
		err := h.db.QueryRow("SELECT id, email, password_hash, role FROM users WHERE email = $1", req.Email).Scan(&user.ID, &user.Email, &user.Password, &user.Role)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		// 3. WRITE BACK TO CACHE (TTL: 1 Hour)
		if jsonBytes, err := json.Marshal(user); err == nil {
			h.redisClient.Set(context.Background(), cacheKey, jsonBytes, time.Hour)
		}
	}

	// 4. VERIFY PASSWORD (CPU Intensive)
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// 5. GENERATE TOKEN
	expiresIn := time.Hour * 24
	expirationTime := time.Now().Add(expiresIn)

	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"exp":     expirationTime.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": dto.LoginResponse{
			Token:     tokenString,
			ExpiresIn: int64(expiresIn.Seconds()),
			UserID:    user.ID,
			Email:     user.Email,
			Role:      user.Role,
		},
	})
}