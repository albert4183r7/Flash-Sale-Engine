// Package publisher provides AWS SQS publishing functionality for async order processing.
//
// Architecture Overview:
// The Flash Sale system uses Amazon SQS for eventual consistency. When a user makes
// a purchase, we immediately reserve stock in Redis and publish an OrderEvent to
// SQS. The order-worker then consumes these events asynchronously to:
//  1. Create the order in PostgreSQL
//  2. Update order status to SUCCESS
//
// This decouples the fast purchase path from slower database operations,
// allowing the system to handle thousands of concurrent purchases.
package publisher

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
)

// Note: OrderEvent is defined in rabbitmq.go to avoid duplication

// SQSPublisher wraps AWS SQS client
type SQSPublisher struct {
	client   *sqs.Client
	queueURL string
}

// NewSQSPublisher creates a new SQS publisher
func NewSQSPublisher(queueURL string) (*SQSPublisher, error) {
	ctx := context.Background()

	// Load AWS configuration from environment (uses IRSA in EKS)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := sqs.NewFromConfig(cfg)

	// Verify queue exists by getting attributes
	_, err = client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{"QueueArn"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to verify SQS queue: %w", err)
	}

	log.Printf("Connected to SQS queue: %s", queueURL)
	return &SQSPublisher{
		client:   client,
		queueURL: queueURL,
	}, nil
}

// Close is a no-op for SQS (no persistent connection)
func (p *SQSPublisher) Close() {
	// SQS uses HTTP, no connection to close
}

// PublishOrderCreated publishes an order.created event to SQS
func (p *SQSPublisher) PublishOrderCreated(event OrderEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	result, err := p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(p.queueURL),
		MessageBody:            aws.String(string(body)),
		MessageDeduplicationId: aws.String(event.OrderID.String()), // For FIFO queues (optional)
		MessageGroupId:         aws.String("orders"),               // For FIFO queues (optional)
	})
	if err != nil {
		return fmt.Errorf("failed to send message to SQS: %w", err)
	}

	log.Printf("Published order.created event to SQS: OrderID=%s, MessageID=%s",
		event.OrderID, *result.MessageId)

	return nil
}
