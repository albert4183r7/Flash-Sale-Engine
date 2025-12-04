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
	// 1. Silently attempt to load .env
	_ = godotenv.Load("../.env")
	_ = godotenv.Load()

	// 2. Get Database URL
	dbURL := os.Getenv("POSTGRES_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable"
	}

	// 3. Connect to Database
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open DB connection: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping DB: %v", err)
	}

	fmt.Println("Connected to Database. Starting seed...")

	// 4. Seed Products (Random UUIDs)
	products := []struct {
		Name  string
		Price int
		Stock int
	}{
		{"iPhone 15 Pro", 999, 100},
		{"MacBook Air M3", 1299, 8},
		{"AirPods Pro", 249, 200},
		{"iPad Pro", 799, 75},
		{"Apple Watch", 399, 150},
	}

	for _, p := range products {
		// We use Name as the conflict target to avoid duplicates if you run seed twice
		// Note: This assumes you add a UNIQUE constraint on 'name' or just accept duplicates for dev
		// For a clean seed, usually we truncate tables first. 
		
		query := `
			INSERT INTO products (name, price, stock) 
			VALUES ($1, $2, $3)`
		
		_, err := db.Exec(query, p.Name, p.Price, p.Stock)
		if err != nil {
			log.Printf("Error seeding product %s: %v", p.Name, err)
		} else {
			fmt.Printf("   - Seeded Product: %s (Random UUID)\n", p.Name)
		}
	}

	// 5. Seed Users (Random UUIDs)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	users := []struct {
		Email string
		Role  string
	}{
		{"user@example.com", "user"},
		{"admin@example.com", "admin"},
		{"tester@example.com", "tester"},
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