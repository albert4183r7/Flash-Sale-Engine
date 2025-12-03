package models

// User represents a user in the system
type User struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	Password string `json:"-"`
	Role     string `json:"role"`
}

// DummyUsers provides sample users for authentication
// In production, this would come from a database
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
