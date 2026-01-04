package dto

import "github.com/google/uuid"

type SignupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresIn int64     `json:"expires_in"`
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
}

type UserResponse struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Role  string    `json:"role"`
}

type ValidateTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

type ValidateTokenResponse struct {
	Valid  bool      `json:"valid"`
	UserID uuid.UUID `json:"user_id,omitempty"`
	Email  string    `json:"email,omitempty"`
	Role   string    `json:"role,omitempty"`
	Error  string    `json:"error,omitempty"`
}
