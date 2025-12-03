package main

import (
	"database/sql"
	"log"

	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/router"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// 1. Load Environment Variables
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	log.Println("Starting API Gateway...")
	cfg := config.Load()

	// 2. Connect to Database (For Auth & Order lookups)
	db, err := sql.Open("postgres", cfg.PostgresURL)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping DB: %v", err)
	}

	// 3. Setup Router
	r := router.Setup(cfg, db)

	log.Printf("API Gateway listening on port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}