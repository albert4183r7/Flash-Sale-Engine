package dto

import "github.com/google/uuid"

// LoginRequest represents the login request body
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Token     string 		`json:"token"`
	ExpiresIn int64  		`json:"expires_in"`
	UserID    uuid.UUID    	`json:"user_id"`
	Email     string 		`json:"email"`
	Role      string 		`json:"role"`
}
