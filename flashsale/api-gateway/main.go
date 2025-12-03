package main

import (
	"log"

	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/router"
)

func main() {
	log.Println("Starting API Gateway...")

	cfg := config.Load()
	log.Printf("Configuration loaded: Port=%s, PurchaseServiceURL=%s", cfg.Port, cfg.PurchaseServiceURL)

	r := router.Setup(cfg)

	log.Printf("API Gateway listening on port %s", cfg.Port)
	log.Println("Available endpoints:")
	log.Println("  POST /auth/login    - Get JWT token")
	log.Println("  POST /purchase      - Submit purchase (requires JWT)")
	log.Println("  GET  /orders/:id    - Get order status (requires JWT)")
	log.Println("  GET  /health        - Health check")

	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
