package models

// Product represents a product in the flash sale
type Product struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
	Stock int    `json:"stock"`
}

// DummyProducts provides sample products for the flash sale
// In production, stock would be managed in Redis
var DummyProducts = map[int]Product{
	1: {ID: 1, Name: "iPhone 15 Pro", Price: 999, Stock: 100},
	2: {ID: 2, Name: "MacBook Air M3", Price: 1299, Stock: 50},
	3: {ID: 3, Name: "AirPods Pro", Price: 249, Stock: 200},
	4: {ID: 4, Name: "iPad Pro", Price: 799, Stock: 75},
	5: {ID: 5, Name: "Apple Watch", Price: 399, Stock: 150},
}
