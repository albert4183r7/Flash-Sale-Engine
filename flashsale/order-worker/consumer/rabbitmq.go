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

// ... existing structs ...
const (
	exchangeName = "flashsale"
	queueName    = "orders.queue"
	routingKey   = "order.created"
)

type OrderEvent struct {
	OrderID   uuid.UUID `json:"order_id"`
	UserID    int       `json:"user_id"`
	ProductID int       `json:"product_id"`
	Qty       int       `json:"qty"`
	Timestamp time.Time `json:"timestamp"`
}

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

func (c *RabbitMQConsumer) processMessage(msg amqp.Delivery) {
	var event OrderEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		log.Printf("Failed to unmarshal: %v", err)
		msg.Nack(false, false)
		return
	}

	order := &repository.Order{
		ID:        event.OrderID,
		UserID:    event.UserID,
		ProductID: event.ProductID,
		Qty:       event.Qty,
		Status:    repository.OrderStatusPending,
		CreatedAt: event.Timestamp,
	}

	// Create Order
	if err := c.orderRepo.Create(order); err != nil {
		log.Printf("DB Error Create: %v", err)
		msg.Nack(false, true)
		return
	}

	// SYNC: Update Product Stock in DB (Eventual Consistency)
	if err := c.orderRepo.DecreaseProductStock(event.ProductID, event.Qty); err != nil {
		log.Printf("DB Error Stock Sync: %v", err)
		// We still acknowledge the order creation, as stock is primarily managed in Redis.
		// In a real system, you might flag this for reconciliation.
	}

	// Update Status to Success
	if err := c.orderRepo.UpdateStatus(event.OrderID, repository.OrderStatusSuccess); err != nil {
		log.Printf("Failed to update status: %v", err)
	}

	msg.Ack(false)
}