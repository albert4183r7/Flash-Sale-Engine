package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/purchase-service/config"
	"github.com/flashsale/purchase-service/handler"
	"github.com/flashsale/purchase-service/publisher"
	"github.com/flashsale/purchase-service/redis"
	"github.com/flashsale/purchase-service/service"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file, using system env")
	}

	cfg := config.Load()
	log.Println("Starting Purchase Service...")

	// 1. Setup Redis
	redisClient := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword)
	defer redisClient.Close()

	// 2. Setup RabbitMQ
	rabbitPublisher, err := publisher.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Printf("Error connecting to RabbitMQ: %v", err)
	} else {
		defer rabbitPublisher.Close()
	}

	// 3. WARM UP: Load Stock from DB to Redis
	db, err := sql.Open("postgres", os.Getenv("POSTGRES_URL"))
	if err != nil {
		log.Fatalf("DB Connect Error: %v", err)
	}
	
	// Retry loop for DB connection during container startup
	for i := 0; i < 5; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	rows, err := db.Query("SELECT id, stock FROM products")
	if err != nil {
		log.Printf("Warning: Failed to fetch products for warmup: %v", err)
	} else {
		defer rows.Close()
		ctx := context.Background()
		for rows.Next() {
			var id, stock int
			if err := rows.Scan(&id, &stock); err == nil {
				redisClient.InitializeStock(ctx, id, stock)
				log.Printf("Warmed up Product %d with Stock %d", id, stock)
			}
		}
	}
	db.Close() // Close DB connection after warmup, Purchase Service runs on Redis

	purchaseService := service.NewPurchaseService(redisClient, rabbitPublisher, cfg.IdempotencyTTL)

	// 4. Setup Router
	r := gin.New()
	r.Use(gin.Recovery())
	
	purchaseHandler := handler.NewPurchaseHandler(purchaseService)
	stockHandler := handler.NewStockHandler(redisClient)

	r.POST("/purchase", purchaseHandler.Purchase)
	r.GET("/stock/:id", stockHandler.GetStock)
	// Internal endpoint for cancellation
	r.POST("/internal/stock/restore", stockHandler.RestoreStock) 

	go func() {
		r.Run(":" + cfg.Port)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}