package consumer

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/flashsale/order-worker/client"
	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

const (
	exchangeName = "flashsale"
	queueName    = "orders.queue"
	routingKey   = "order.created"
)

type OrderEvent struct {
	OrderID       uuid.UUID `json:"order_id"`
	UserID        uuid.UUID `json:"user_id"`
	ProductID     uuid.UUID `json:"product_id"`
	Qty           int       `json:"qty"`
	Notes         string    `json:"notes,omitempty"`
	PaymentMethod string    `json:"payment_method,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

type RabbitMQConsumer struct {
	conn          *amqp.Connection
	channel       *amqp.Channel
	orderClient   *client.OrderServiceClient
	productClient *client.ProductServiceClient
	paymentClient *client.PaymentServiceClient
}

// NewRabbitMQConsumer creates a new RabbitMQ consumer with HTTP clients
func NewRabbitMQConsumer(url string, orderClient *client.OrderServiceClient, productClient *client.ProductServiceClient, paymentClient *client.PaymentServiceClient) (*RabbitMQConsumer, error) {
	var conn *amqp.Connection
	var err error

	for i := 0; i < 5; i++ {
		conn, err = amqp.Dial(url)
		if err == nil {
			break
		}
		log.Printf("Failed to connect to RabbitMQ (attempt %d/5): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ after 5 attempts: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	err = channel.ExchangeDeclare(
		exchangeName,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	_, err = channel.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	err = channel.QueueBind(
		queueName,
		routingKey,
		exchangeName,
		false,
		nil,
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	log.Println("Connected to RabbitMQ successfully")
	return &RabbitMQConsumer{
		conn:          conn,
		channel:       channel,
		orderClient:   orderClient,
		productClient: productClient,
		paymentClient: paymentClient,
	}, nil
}

// Close closes the RabbitMQ connection
func (c *RabbitMQConsumer) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// Start begins consuming messages from the queue
func (c *RabbitMQConsumer) Start() error {
	err := c.channel.Qos(1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	msgs, err := c.channel.Consume(
		queueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	log.Printf("Order Worker started. Waiting for messages on queue: %s", queueName)

	for msg := range msgs {
		c.processMessage(msg)
	}

	return nil
}

func (c *RabbitMQConsumer) processMessage(msg amqp.Delivery) {
	var event OrderEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		log.Printf("Failed to unmarshal: %v", err)
		msg.Nack(false, false)
		return
	}

	log.Printf("Processing order: %s for product: %s", event.OrderID, event.ProductID)

	// 1. Fetch product info from Product Service
	product, err := c.productClient.GetProduct(event.ProductID)
	if err != nil {
		log.Printf("Failed to get product info: %v", err)
		msg.Nack(false, true) // Requeue
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
		msg.Nack(false, true) // Requeue
		return
	}

	if !result.Success {
		log.Printf("Failed to create order: %s", result.Error)
		msg.Nack(false, false) // Don't requeue
		return
	}

	// 3. Process Payment via Payment Service
	paymentMethod := event.PaymentMethod
	if paymentMethod == "" {
		paymentMethod = "mock_success" // Default to success for testing
	}
	
	totalAmount := product.Price * event.Qty
	paymentResult, err := c.paymentClient.ProcessPayment(event.OrderID, totalAmount, paymentMethod)
	if err != nil {
		log.Printf("Payment Service Error: %v", err)
		// Update order to FAILED
		c.orderClient.UpdateOrderStatus(event.OrderID, "FAILED")
		msg.Ack(false)
		return
	}

	// 4. Update Order Status based on payment result
	var orderStatus string
	if paymentResult.Success {
		orderStatus = "SUCCESS"
		// Sync stock to DB (eventual consistency)
		if err := c.productClient.SyncStockToDB(event.ProductID, event.Qty); err != nil {
			log.Printf("Product Service Stock Sync Error: %v", err)
		}
	} else {
		orderStatus = "FAILED"
		log.Printf("Payment failed for order %s: %s", event.OrderID, paymentResult.Message)
	}

	if err := c.orderClient.UpdateOrderStatus(event.OrderID, orderStatus); err != nil {
		log.Printf("Failed to update order status: %v", err)
	}

	log.Printf("Order %s processed with status: %s (Payment: %s)", event.OrderID, orderStatus, paymentResult.Status)
	msg.Ack(false)
}