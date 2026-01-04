// Package consumer provides AWS SQS consuming functionality for async order processing.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/flashsale/order-worker/client"
)

// Note: OrderEvent is defined in rabbitmq.go to avoid duplication

type SQSConsumer struct {
	client        *sqs.Client
	queueURL      string
	orderClient   *client.OrderServiceClient
	productClient *client.ProductServiceClient
	stopCh        chan struct{}
}

// NewSQSConsumer creates a new SQS consumer with HTTP clients
func NewSQSConsumer(queueURL string, orderClient *client.OrderServiceClient, productClient *client.ProductServiceClient) (*SQSConsumer, error) {
	ctx := context.Background()

	// Load AWS configuration from environment (uses IRSA in EKS)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	sqsClient := sqs.NewFromConfig(cfg)

	// Verify queue exists
	_, err = sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{"QueueArn"},
	})
	if err != nil {
		return nil, err
	}

	log.Printf("Connected to SQS queue: %s", queueURL)
	return &SQSConsumer{
		client:        sqsClient,
		queueURL:      queueURL,
		orderClient:   orderClient,
		productClient: productClient,
		stopCh:        make(chan struct{}),
	}, nil
}

// Close signals the consumer to stop
func (c *SQSConsumer) Close() {
	close(c.stopCh)
}

// Start begins consuming messages from SQS using long polling
func (c *SQSConsumer) Start() error {
	log.Printf("Order Worker started. Polling SQS queue: %s", c.queueURL)

	for {
		select {
		case <-c.stopCh:
			log.Println("SQS consumer stopping...")
			return nil
		default:
			c.pollMessages()
		}
	}
}

func (c *SQSConsumer) pollMessages() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(c.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     20, // Long polling
		VisibilityTimeout:   60, // 60 seconds to process
	})
	if err != nil {
		log.Printf("Failed to receive messages: %v", err)
		time.Sleep(2 * time.Second)
		return
	}

	for _, msg := range result.Messages {
		c.processMessage(msg)
	}
}

func (c *SQSConsumer) processMessage(msg types.Message) {
	var event OrderEvent
	if err := json.Unmarshal([]byte(*msg.Body), &event); err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		c.deleteMessage(msg) // Delete malformed messages
		return
	}

	log.Printf("Processing order: %s for product: %s", event.OrderID, event.ProductID)

	// 1. Fetch product info from Product Service
	product, err := c.productClient.GetProduct(event.ProductID)
	if err != nil {
		log.Printf("Failed to get product info: %v", err)
		// Don't delete - will be retried after visibility timeout
		return
	}

	// 2. Create Order via Order Service (with product name and price)
	result, err := c.orderClient.CreateOrder(
		event.OrderID,
		event.UserID,
		event.ProductID,
		product.Name,
		product.Price,
		event.Qty,
		event.Notes,
	)
	if err != nil {
		log.Printf("Order Service Error: %v", err)
		return // Retry
	}

	if !result.Success {
		log.Printf("Failed to create order: %s", result.Error)
		c.deleteMessage(msg) // Delete - don't retry permanent failures
		return
	}

	// 3. Mark order as SUCCESS
	orderStatus := "SUCCESS"

	// 4. Sync stock to DB (eventual consistency)
	if err := c.productClient.SyncStockToDB(event.ProductID, event.Qty); err != nil {
		log.Printf("Product Service Stock Sync Error: %v", err)
	}

	// 5. Update order status
	if err := c.orderClient.UpdateOrderStatus(event.OrderID, orderStatus); err != nil {
		log.Printf("Failed to update order status: %v", err)
	}

	// Success - delete message from queue
	c.deleteMessage(msg)
	log.Printf("Order %s processed with status: %s", event.OrderID, orderStatus)
}

func (c *SQSConsumer) deleteMessage(msg types.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.Printf("Failed to delete message: %v", err)
	}
}
