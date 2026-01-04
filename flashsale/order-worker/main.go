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
	log.Println("Starting Order Worker...")
	cfg := config.Load()

	// Initialize HTTP clients for services
	orderClient := client.NewOrderServiceClient(cfg.OrderServiceURL)
	productClient := client.NewProductServiceClient(cfg.ProductServiceURL)

	sqsQueueURL := os.Getenv("SQS_QUEUE_URL")
	
	if sqsQueueURL != "" {
		// Use SQS in AWS environment
		log.Println("Using Amazon SQS for messaging")
		sqsConsumer, err := consumer.NewSQSConsumer(sqsQueueURL, orderClient, productClient)
		if err != nil {
			log.Fatalf("Failed to create SQS consumer: %v", err)
		}
		defer sqsConsumer.Close()

		go func() {
			if err := sqsConsumer.Start(); err != nil {
				log.Fatalf("SQS Consumer error: %v", err)
			}
		}()
	} else {
		// Fallback to RabbitMQ for local development
		log.Println("Using RabbitMQ for messaging (local development)")
		rabbitConsumer, err := consumer.NewRabbitMQConsumer(cfg.RabbitMQURL, orderClient, productClient)
		if err != nil {
			log.Fatalf("Failed to create RabbitMQ consumer: %v", err)
		}
		defer rabbitConsumer.Close()

		go func() {
			if err := rabbitConsumer.Start(); err != nil {
				log.Fatalf("RabbitMQ Consumer error: %v", err)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Order Worker...")
}