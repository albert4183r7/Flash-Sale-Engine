package models

import "github.com/google/uuid"

// User represents an account that can place orders.
type User struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Role  string    `json:"role"`
}
