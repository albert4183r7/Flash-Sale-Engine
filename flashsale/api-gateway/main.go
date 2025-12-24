package main

import (
	"log"

	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/router"
	"github.com/joho/godotenv"
)

func main() {
	// 1. Load Environment Variables
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	log.Println("Starting API Gateway...")
	cfg := config.Load()

	// 2. Setup Router (no DB or Redis connections - pure proxy)
	r := router.Setup(cfg)

	log.Printf("API Gateway listening on port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}