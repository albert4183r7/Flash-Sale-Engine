// Package handler contains the API gateway's HTTP handlers. Handlers parse and
// validate requests, delegate to the service layer and render responses; the
// business rules live in the services.
package handler

import (
	"errors"
	"net/http"

	"github.com/flashsale/api-gateway/dto"
	"github.com/flashsale/api-gateway/service"
	"github.com/flashsale/common/response"
	"github.com/gin-gonic/gin"
)

// AuthHandler serves registration and login.
type AuthHandler struct {
	auth *service.Auth
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(auth *service.Auth) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// Signup handles POST /auth/signup.
func (h *AuthHandler) Signup(c *gin.Context) {
	var req dto.SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	err := h.auth.Register(c.Request.Context(), req.Email, req.Password)
	switch {
	case err == nil:
		response.Success(c, http.StatusCreated, "User registered successfully", nil)
	case errors.Is(err, service.ErrEmailTaken):
		response.Error(c, http.StatusConflict, "Registration failed",
			"An account with this email already exists")
	default:
		// The underlying error is logged by the service; the client is told only
		// that the request failed, so internal details are not disclosed.
		response.Error(c, http.StatusInternalServerError, "Registration failed",
			"Unable to register the account")
	}
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	token, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
	switch {
	case err == nil:
		response.Success(c, http.StatusOK, "Login successful", dto.LoginResponse{
			Token:     token.Value,
			ExpiresIn: int64(token.ExpiresIn.Seconds()),
			UserID:    token.User.ID,
			Email:     token.User.Email,
			Role:      token.User.Role,
		})
	case errors.Is(err, service.ErrInvalidCredentials):
		// The same answer for an unknown email and a wrong password, so the API
		// cannot be used to discover which addresses are registered.
		response.Error(c, http.StatusUnauthorized, "Login failed", "Invalid credentials")
	default:
		response.Error(c, http.StatusInternalServerError, "Login failed",
			"Unable to complete the login")
	}
}
