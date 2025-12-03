package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/flashsale/purchase-service/config"
	"github.com/flashsale/purchase-service/handler"
	"github.com/flashsale/purchase-service/publisher"
	"github.com/flashsale/purchase-service/redis"
	"github.com/flashsale/purchase-service/service"
	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting Purchase Service...")

	cfg := config.Load()
	log.Printf("Configuration loaded: Port=%s, RedisAddr=%s", cfg.Port, cfg.RedisAddr)

	redisClient := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword)
	defer redisClient.Close()

	rabbitPublisher, err := publisher.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Printf("Warning: Could not connect to RabbitMQ: %v", err)
		log.Println("Purchase Service will start but order publishing will fail")
	} else {
		defer rabbitPublisher.Close()
	}

	purchaseService := service.NewPurchaseService(redisClient, rabbitPublisher, cfg.IdempotencyTTL)

	if err := purchaseService.InitializeProducts(context.Background()); err != nil {
		log.Printf("Warning: Failed to initialize products: %v", err)
	} else {
		log.Println("Product stock initialized in Redis")
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	healthHandler := handler.NewHealthHandler()
	purchaseHandler := handler.NewPurchaseHandler(purchaseService)
	stockHandler := handler.NewStockHandler(redisClient)

	r.GET("/health", healthHandler.Health)
	r.POST("/purchase", purchaseHandler.Purchase)
	r.GET("/stock/:id", stockHandler.GetStock)

	go func() {
		log.Printf("Purchase Service listening on port %s", cfg.Port)
		log.Println("Available endpoints:")
		log.Println("  POST /purchase     - Process purchase")
		log.Println("  GET  /stock/:id    - Get product stock")
		log.Println("  GET  /health       - Health check")

		if err := r.Run(":" + cfg.Port); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Purchase Service...")
}
