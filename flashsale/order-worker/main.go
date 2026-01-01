package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/flashsale/order-worker/client"
	"github.com/flashsale/order-worker/config"
	"github.com/flashsale/order-worker/consumer"
)

func main() {
	// Note: In production (GKE), env vars come from ConfigMap/Secrets
	// godotenv is removed for cloud deployment
	log.Println("Starting Order Worker...")
	cfg := config.Load()

	// Initialize HTTP clients for services
	orderClient := client.NewOrderServiceClient(cfg.OrderServiceURL)
	productClient := client.NewProductServiceClient(cfg.ProductServiceURL)

	// Create RabbitMQ consumer with HTTP clients
	rabbitConsumer, err := consumer.NewRabbitMQConsumer(cfg.RabbitMQURL, orderClient, productClient)
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
	log.Println("Shutting down Order Worker...")
}