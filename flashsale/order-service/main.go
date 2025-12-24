package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/order-service/config"
	"github.com/flashsale/order-service/handler"
	"github.com/flashsale/order-service/repository"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	log.Println("Starting Order Service...")
	cfg := config.Load()

	// 1. Connect to PostgreSQL (Orders DB)
	db, err := sql.Open("postgres", cfg.PostgresURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Retry loop for DB connection
	for i := 0; i < 5; i++ {
		if err := db.Ping(); err == nil {
			log.Println("Connected to PostgreSQL successfully")
			break
		}
		log.Printf("Waiting for database... (attempt %d/5)", i+1)
		time.Sleep(2 * time.Second)
	}

	// 2. Initialize Repository and Handlers
	orderRepo := repository.NewOrderRepository(db)
	orderHandler := handler.NewOrderHandler(orderRepo)
	healthHandler := handler.NewHealthHandler()

	// 3. Setup Router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Health check
	r.GET("/health", healthHandler.Health)

	// Order routes
	r.GET("/orders/:id", orderHandler.GetOrder)
	r.GET("/users/:user_id/orders", orderHandler.GetOrdersByUser)

	// Internal routes (for other services)
	internal := r.Group("/internal")
	{
		internal.POST("/orders", orderHandler.CreateOrder)
		internal.PATCH("/orders/:id/status", orderHandler.UpdateOrderStatus)
		internal.DELETE("/orders/:id", orderHandler.CancelOrder)
	}

	// 4. Start server
	go func() {
		log.Printf("Order Service listening on port %s", cfg.Port)
		if err := r.Run(":" + cfg.Port); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 5. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Order Service...")
}
