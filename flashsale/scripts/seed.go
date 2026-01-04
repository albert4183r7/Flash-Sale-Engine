package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Only load .env.seed if it exists (for custom config)
	// Otherwise use localhost defaults for host machine seeding
	_ = godotenv.Load(".env.seed")

	fmt.Println("=== Flash Sale Database Seeder (Microservices) ===")
	fmt.Println("Connecting to databases on localhost (Docker exposed ports)...")

	// 2. Seed Users Database
	seedUsers()

	// 3. Seed Products Database
	seedProducts()

	// 4. Warm up Redis cache with stock data
	warmupCache()

	fmt.Println("\n✅ All databases seeded and cache warmed up!")
}

func seedUsers() {
	userDBURL := os.Getenv("USER_POSTGRES_URL")
	if userDBURL == "" {
		// Default for Docker Compose - password is 'postgres'
		userDBURL = "postgres://postgres:postgres@localhost:5433/flashsale_users?sslmode=disable"
	}

	db, err := sql.Open("postgres", userDBURL)
	if err != nil {
		log.Printf("Warning: Could not connect to Users DB: %v", err)
		return
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Printf("Warning: Users DB not reachable: %v", err)
		return
	}

	fmt.Println("\n📦 Seeding Users Database...")

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
			log.Printf("   ❌ Error seeding user %s: %v", u.Email, err)
		} else {
			fmt.Printf("   ✓ Seeded User: %s (%s)\n", u.Email, u.Role)
		}
	}
}

func seedProducts() {
	productDBURL := os.Getenv("PRODUCT_POSTGRES_URL")
	if productDBURL == "" {
		productDBURL = "postgres://postgres:postgres@localhost:5434/flashsale_products?sslmode=disable"
	}

	db, err := sql.Open("postgres", productDBURL)
	if err != nil {
		log.Printf("Warning: Could not connect to Products DB: %v", err)
		return
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Printf("Warning: Products DB not reachable: %v", err)
		return
	}

	fmt.Println("\n📦 Seeding Products Database...")

	products := []struct {
		Name        string
		Description string
		Price       int
		Stock       int
	}{
		{
			"iPhone 15 Pro",
			"Latest flagship smartphone with A17 Pro chip, titanium design. Available in Natural Titanium, Blue Titanium, White Titanium, Black Titanium. Storage: 128GB, 256GB, 512GB, 1TB.",
			999, 100,
		},
		{
			"MacBook Air M3",
			"Ultra-thin laptop with M3 chip, 15.3-inch Liquid Retina display. Available in Midnight, Starlight, Space Gray, Silver. RAM: 8GB, 16GB, 24GB. Storage: 256GB, 512GB, 1TB, 2TB.",
			1299, 8,
		},
		{
			"AirPods Pro",
			"Wireless earbuds with Active Noise Cancellation, Adaptive Audio, USB-C charging case. Available in White only.",
			249, 200,
		},
		{
			"iPad Pro",
			"Professional tablet with M2 chip, Liquid Retina XDR display. Available in Space Gray, Silver. Size: 11-inch, 12.9-inch. Storage: 128GB, 256GB, 512GB, 1TB, 2TB.",
			799, 75,
		},
		{
			"Apple Watch",
			"Smart watch with health monitoring, GPS, cellular connectivity. Available in Midnight, Starlight, Silver, Red, Blue. Size: 41mm, 45mm.",
			399, 150,
		},
	}

	for _, p := range products {
		// First check if product exists
		var count int
		db.QueryRow("SELECT COUNT(*) FROM products WHERE name = $1", p.Name).Scan(&count)
		
		if count > 0 {
			// Update existing product with description
			query := `UPDATE products SET description = $1 WHERE name = $2`
			db.Exec(query, p.Description, p.Name)
			fmt.Printf("   ⏭ Product updated: %s\n", p.Name)
			continue
		}

		query := `INSERT INTO products (name, description, price, stock) VALUES ($1, $2, $3, $4)`
		_, err := db.Exec(query, p.Name, p.Description, p.Price, p.Stock)
		if err != nil {
			log.Printf("   ❌ Error seeding product %s: %v", p.Name, err)
		} else {
			fmt.Printf("   ✓ Seeded Product: %s (Stock: %d, Price: $%d)\n", p.Name, p.Stock, p.Price)
		}
	}
}

func warmupCache() {
	fmt.Println("\n🔥 Warming up Redis cache...")

	// Call product-service warmup endpoint
	resp, err := http.Post("http://localhost:8083/internal/stock/warmup", "application/json", nil)
	if err != nil {
		log.Printf("   ⚠️ Warning: Could not warm up cache: %v", err)
		fmt.Println("   Run manually: curl -X POST http://localhost:8083/internal/stock/warmup")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("   ✓ Redis cache warmed up successfully!")
	} else {
		log.Printf("   ⚠️ Warning: Warmup returned status %d", resp.StatusCode)
	}
}