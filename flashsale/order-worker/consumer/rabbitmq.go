package consumer

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/flashsale/order-worker/repository"
	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

const (
	exchangeName = "flashsale"
	queueName    = "orders.queue"
	routingKey   = "order.created"
)

// OrderEvent represents the message received from RabbitMQ
type OrderEvent struct {
	OrderID   uuid.UUID `json:"order_id"`
	UserID    int       `json:"user_id"`
	ProductID int       `json:"product_id"`
	Qty       int       `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}

// RabbitMQConsumer consumes messages from RabbitMQ
type RabbitMQConsumer struct {
	conn        *amqp.Connection
	channel     *amqp.Channel
	orderRepo   *repository.OrderRepository
}

// NewRabbitMQConsumer creates a new RabbitMQ consumer
func NewRabbitMQConsumer(url string, orderRepo *repository.OrderRepository) (*RabbitMQConsumer, error) {
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
		conn:      conn,
		channel:   channel,
		orderRepo: orderRepo,
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

// processMessage handles a single message
func (c *RabbitMQConsumer) processMessage(msg amqp.Delivery) {
	var event OrderEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		msg.Nack(false, false)
		return
	}

	log.Printf("Processing order: OrderID=%s, UserID=%d, ProductID=%d, Qty=%d",
		event.OrderID, event.UserID, event.ProductID, event.Qty)

	order := &repository.Order{
		ID:        event.OrderID,
		UserID:    event.UserID,
		ProductID: event.ProductID,
		Qty:       event.Qty,
		Status:    repository.OrderStatusPending,
		CreatedAt: event.Timestamp,
	}

	if err := c.orderRepo.Create(order); err != nil {
		log.Printf("Failed to create order: %v", err)

		if err := c.orderRepo.UpdateStatus(event.OrderID, repository.OrderStatusFailed); err != nil {
			log.Printf("Failed to update order status to FAILED: %v", err)
		}

		msg.Nack(false, true)
		return
	}

	if err := c.orderRepo.UpdateStatus(event.OrderID, repository.OrderStatusSuccess); err != nil {
		log.Printf("Failed to update order status to SUCCESS: %v", err)
		msg.Nack(false, true)
		return
	}

	log.Printf("Order processed successfully: OrderID=%s, Status=SUCCESS", event.OrderID)
	msg.Ack(false)
}
