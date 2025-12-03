package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// 1. Silently attempt to load .env from parent or current directory
	_ = godotenv.Load("../.env")
	_ = godotenv.Load()

	// 2. Get Database URL with a Hardcoded Fallback
	dbURL := os.Getenv("POSTGRES_URL")
	if dbURL == "" {
		// Default to the standard Docker Compose setup
		dbURL = "postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable"
		fmt.Printf("POSTGRES_URL not set. Using default: %s\n", dbURL)
	}

	// 3. Connect to Database
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open DB connection: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping DB. Check if Docker/Postgres is running. Error: %v", err)
	}

	fmt.Println("Connected to Database. Starting seed...")

	// 4. Seed Products
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
		query := `
			INSERT INTO products (id, name, price, stock) 
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE 
			SET stock = $4, price = $3`
		
		_, err := db.Exec(query, p.ID, p.Name, p.Price, p.Stock)
		if err != nil {
			log.Printf("Error seeding product %s: %v", p.Name, err)
		} else {
			fmt.Printf("   - Seeded Product: %s\n", p.Name)
		}
	}

	// 5. Seed Users
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	users := []struct {
		Email string
		Role  string
	}{
		{"user@example.com", "user"},
		{"admin@example.com", "admin"},
		{"buyer@example.com", "user"},
		{"tester@example.com", "user"},
	}

	for _, u := range users {
		query := `
			INSERT INTO users (email, password_hash, role) 
			VALUES ($1, $2, $3)
			ON CONFLICT (email) DO NOTHING`

		_, err := db.Exec(query, u.Email, string(hash), u.Role)
		if err != nil {
			log.Printf("Error seeding user %s: %v", u.Email, err)
		} else {
			fmt.Printf("   - Seeded User: %s\n", u.Email)
		}
	}

	fmt.Println("Database seeded successfully!")
}