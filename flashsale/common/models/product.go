package models

import "github.com/google/uuid"

// Product represents a product in the flash sale
type Product struct {
	ID    uuid.UUID    	`json:"id"`
	Name  string 		`json:"name"`
	Price int    		`json:"price"`
	Stock int    		`json:"stock"`
}
