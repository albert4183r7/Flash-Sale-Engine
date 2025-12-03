package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/flashsale/order-worker/config"
	"github.com/flashsale/order-worker/consumer"
	"github.com/flashsale/order-worker/postgres"
	"github.com/flashsale/order-worker/repository"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found")
	}

	log.Println("Starting Order Worker...")
	cfg := config.Load()
	log.Printf("🛠️  DEBUG: Loaded RabbitMQ URL: %s", cfg.RabbitMQURL)
    log.Printf("🛠️  DEBUG: Loaded Postgres URL: %s", cfg.PostgresURL)

	pgClient, err := postgres.NewClient(cfg.PostgresURL)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pgClient.Close()
    
    // Schema is now handled by init.sql or migration scripts
	// if err := pgClient.InitializeSchema(); err != nil { ... }

	orderRepo := repository.NewOrderRepository(pgClient.DB())

	rabbitConsumer, err := consumer.NewRabbitMQConsumer(cfg.RabbitMQURL, orderRepo)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ consumer: %v", err)
	}
	defer rabbitConsumer.Close()

	go func() {
		if err := rabbitConsumer.Start(); err != nil {
			log.Fatalf("Consumer error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}