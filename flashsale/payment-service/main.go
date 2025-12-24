package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/flashsale/payment-service/handler"
	"github.com/flashsale/payment-service/repository"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func main() {
	// Get configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}

	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		postgresURL = "postgres://postgres:postgres@localhost:5436/flashsale_payments?sslmode=disable"
	}

	// Connect to database with retry
	var db *sql.DB
	var err error
	for i := 0; i < 10; i++ {
		db, err = sql.Open("postgres", postgresURL)
		if err == nil {
			if err = db.Ping(); err == nil {
				break
			}
		}
		log.Printf("Waiting for database... (attempt %d/10)", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Println("Connected to database")

	// Initialize repository and handler
	paymentRepo := repository.NewPaymentRepository(db)
	paymentHandler := handler.NewPaymentHandler(paymentRepo)

	// Setup router
	router := gin.Default()

	// Internal endpoints (called by order-worker)
	router.POST("/internal/process", paymentHandler.ProcessPayment)

	// Public endpoints
	router.GET("/payments/:id", paymentHandler.GetPayment)
	router.GET("/payments/order/:order_id", paymentHandler.GetPaymentByOrder)

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy", "service": "payment-service"})
	})

	log.Printf("Payment Service listening on port %s", port)
	router.Run(fmt.Sprintf(":%s", port))
}
