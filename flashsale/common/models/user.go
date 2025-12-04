package models

import "github.com/google/uuid"

// User represents a user in the system
type User struct {
	ID       uuid.UUID  `json:"id"`
	Email    string 	`json:"email"`
	Password string 	`json:"-"`
	Role     string		`json:"role"`
}