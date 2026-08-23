package models

import "github.com/google/uuid"

// Product represents an item offered in the flash sale.
type Product struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Price int       `json:"price"`
	Stock int       `json:"stock"`
}
