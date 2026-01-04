package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/purchase-service/client"
	"github.com/flashsale/purchase-service/config"
	"github.com/flashsale/purchase-service/handler"
	"github.com/flashsale/purchase-service/publisher"
	"github.com/flashsale/purchase-service/redis"
	"github.com/flashsale/purchase-service/service"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()
	log.Println("Starting Purchase Service...")

	// 1. Setup Redis
	redisClient := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword)
	defer redisClient.Close()

	// 2. Setup SQS Publisher
	var msgPublisher service.MessagePublisher
	var err error

	sqsQueueURL := os.Getenv("SQS_QUEUE_URL")
	if sqsQueueURL != "" {
		// Use SQS in AWS environment
		msgPublisher, err = publisher.NewSQSPublisher(sqsQueueURL)
		if err != nil {
			log.Fatalf("Failed to create SQS publisher: %v", err)
		}
		log.Println("Using Amazon SQS for messaging")
	} else {
		// Fallback to RabbitMQ for local development
		for i := 0; i < 5; i++ {
			msgPublisher, err = publisher.NewRabbitMQ(cfg.RabbitMQURL)
			if err == nil {
				log.Println("Connected to RabbitMQ (local development)")
				break
			}
			log.Printf("Failed to connect to RabbitMQ (attempt %d/5): %v", i+1, err)
			time.Sleep(2 * time.Second)
		}
		if err != nil {
			log.Printf("Warning: Message queue not available: %v", err)
		}
	}
	defer func() {
		if msgPublisher != nil {
			msgPublisher.Close()
		}
	}()

	// 3. Setup Product Client for product name lookups
	productServiceURL := os.Getenv("PRODUCT_SERVICE_URL")
	productClient := client.NewProductClient(productServiceURL)

	// 4. Warmup: Trigger product-service to load stock into Redis
	if productServiceURL != "" {
		time.Sleep(3 * time.Second)
		warmupURL := productServiceURL + "/internal/stock/warmup"
		resp, err := http.Post(warmupURL, "application/json", nil)
		if err != nil {
			log.Printf("Warning: Failed to trigger stock warmup: %v", err)
		} else {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Println("Stock warmup triggered successfully via product-service")
			} else {
				log.Printf("Warning: Stock warmup returned status %d", resp.StatusCode)
			}
		}
	}

	purchaseService := service.NewPurchaseService(redisClient, msgPublisher, productClient, cfg.IdempotencyTTL)

	// 5. Setup Router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	purchaseHandler := handler.NewPurchaseHandler(purchaseService)
	stockHandler := handler.NewStockHandler(redisClient)

	// Health check for Kubernetes probes
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "purchase-service"})
	})

	r.POST("/purchase", purchaseHandler.Purchase)
	r.GET("/stock/:id", stockHandler.GetStock)
	r.POST("/internal/stock/restore", stockHandler.RestoreStock)

	go func() {
		log.Printf("Purchase Service listening on port %s", cfg.Port)
		r.Run(":" + cfg.Port)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Purchase Service...")
}