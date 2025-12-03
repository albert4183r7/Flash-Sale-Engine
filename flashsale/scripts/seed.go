package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found, using system env")
	}

	dbURL := os.Getenv("POSTGRES_URL")
	if dbURL == "" {
		log.Fatal("POSTGRES_URL is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping DB: %v", err)
	}

	// Seed Products
	products := []struct {
		ID    int
		Name  string
		Price int
		Stock int
	}{
		{1, "iPhone 15 Pro", 999, 100},
		{2, "MacBook Air M3", 1299, 50},
		{3, "AirPods Pro", 249, 200},
		{4, "iPad Pro", 799, 75},
		{5, "Apple Watch", 399, 150},
	}

	for _, p := range products {
		_, err := db.Exec(`
			INSERT INTO products (id, name, price, stock) 
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE 
			SET stock = $4, price = $3`, p.ID, p.Name, p.Price, p.Stock)
		if err != nil {
			log.Printf("Error seeding product %s: %v", p.Name, err)
		}
	}

	// Seed Users
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	users := []struct {
		Email string
		Role  string
	}{
		{"user@example.com", "user"},
		{"admin@example.com", "admin"},
	}

	for _, u := range users {
		_, err := db.Exec(`
			INSERT INTO users (email, password_hash, role) 
			VALUES ($1, $2, $3)
			ON CONFLICT (email) DO NOTHING`, u.Email, string(hash), u.Role)
		if err != nil {
			log.Printf("Error seeding user %s: %v", u.Email, err)
		}
	}

	log.Println("Database seeded successfully!")
}