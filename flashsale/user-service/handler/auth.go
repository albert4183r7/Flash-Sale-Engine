package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/flashsale/user-service/cache"
	"github.com/flashsale/user-service/dto"
	"github.com/flashsale/user-service/repository"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userRepo    *repository.UserRepository
	userCache   *cache.UserCache
	jwtSecret   string
	tokenExpiry int
}

func NewAuthHandler(userRepo *repository.UserRepository, userCache *cache.UserCache, jwtSecret string, tokenExpiry int) *AuthHandler {
	return &AuthHandler{
		userRepo:    userRepo,
		userCache:   userCache,
		jwtSecret:   jwtSecret,
		tokenExpiry: tokenExpiry,
	}
}

func (h *AuthHandler) Signup(c *gin.Context) {
	var req dto.SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user, err := h.userRepo.Create(req.Email, string(hashedPassword))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User registered successfully",
		"user_id": user.ID,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()
	var user *repository.User
	var passwordHash string

	// 1. Check Cache
	cachedUser, err := h.userCache.GetUser(ctx, req.Email)
	if err == nil && cachedUser != nil {
		// Cache hit
		user = &repository.User{
			Email: cachedUser.Email,
			Role:  cachedUser.Role,
		}
		user.ID, _ = uuid.Parse(cachedUser.ID)
		passwordHash = cachedUser.Password
	} else {
		// 2. Cache miss - query database
		user, err = h.userRepo.FindByEmail(req.Email)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		passwordHash = user.PasswordHash

		// 3. Write to cache
		h.userCache.SetUser(ctx, req.Email, &cache.CachedUser{
			ID:       user.ID.String(),
			Email:    user.Email,
			Password: user.PasswordHash,
			Role:     user.Role,
		}, time.Hour)
	}

	// 4. Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// 5. Generate JWT
	expiresIn := time.Hour * time.Duration(h.tokenExpiry)
	expirationTime := time.Now().Add(expiresIn)

	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
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

func (h *AuthHandler) GetUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	user, err := h.userRepo.FindByID(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, dto.UserResponse{
		ID:    user.ID,
		Email: user.Email,
		Role:  user.Role,
	})
}

func (h *AuthHandler) ValidateToken(c *gin.Context) {
	var req dto.ValidateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.jwtSecret), nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusOK, dto.ValidateTokenResponse{
			Valid: false,
			Error: "Invalid or expired token",
		})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusOK, dto.ValidateTokenResponse{
			Valid: false,
			Error: "Invalid token claims",
		})
		return
	}

	userIDStr, _ := claims["user_id"].(string)
	userID, _ := uuid.Parse(userIDStr)
	email, _ := claims["email"].(string)
	role, _ := claims["role"].(string)

	c.JSON(http.StatusOK, dto.ValidateTokenResponse{
		Valid:  true,
		UserID: userID,
		Email:  email,
		Role:   role,
	})
}
