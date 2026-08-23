// Package consumer receives order events from RabbitMQ and hands them to the
// order store for persistence.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/order-worker/repository"
	"github.com/streadway/amqp"
)

const (
	exchangeName = "flashsale"
	queueName    = "orders.queue"
	routingKey   = "order.created"

	connectAttempts = 5
	connectBackoff  = 2 * time.Second

	// retryDelay throttles redelivery after a transient failure. Without it a
	// message that keeps failing is requeued in a tight loop that saturates the
	// worker, the broker and the database.
	retryDelay = 2 * time.Second
)

// ErrConnectionClosed reports that the broker closed the delivery stream. The
// worker exits on this so its supervisor can restart it with a fresh
// connection, rather than lingering as a process that consumes nothing.
var ErrConnectionClosed = errors.New("rabbitmq delivery channel closed")

// OrderStore persists an order event. It is an interface so the consumer's
// message handling can be tested without a database.
type OrderStore interface {
	Persist(ctx context.Context, event models.OrderEvent) (models.OrderStatus, error)
}

// Consumer reads order events from RabbitMQ.
type Consumer struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	store     OrderStore
	dbTimeout time.Duration
}

// New dials RabbitMQ, declares the topology and returns a ready consumer.
func New(url string, store OrderStore, dbTimeout time.Duration) (*Consumer, error) {
	conn, err := dial(url)
	if err != nil {
		return nil, err
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	if err := declareTopology(channel); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	// Process one message at a time so a restart loses at most one message and
	// the database is not flooded during a spike.
	if err := channel.Qos(1, 0, false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("set QoS: %w", err)
	}

	log.Println("connected to RabbitMQ")
	return &Consumer{conn: conn, channel: channel, store: store, dbTimeout: dbTimeout}, nil
}

func dial(url string) (*amqp.Connection, error) {
	var conn *amqp.Connection
	var err error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		conn, err = amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		log.Printf("rabbitmq not ready (attempt %d/%d): %v", attempt, connectAttempts, err)
		time.Sleep(connectBackoff)
	}
	return nil, fmt.Errorf("connect to rabbitmq after %d attempts: %w", connectAttempts, err)
}

func declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(exchangeName, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", exchangeName, err)
	}
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", queueName, err)
	}
	if err := ch.QueueBind(queueName, routingKey, exchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q: %w", queueName, err)
	}
	return nil
}

// Close releases the broker resources.
func (c *Consumer) Close() {
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

// Run consumes messages until ctx is cancelled or the broker closes the stream.
// It returns nil only on a clean shutdown via ctx.
func (c *Consumer) Run(ctx context.Context) error {
	// A named consumer tag lets us cancel delivery on shutdown while still
	// finishing the message currently being processed.
	const consumerTag = "order-worker"

	deliveries, err := c.channel.Consume(queueName, consumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("start consuming from %q: %w", queueName, err)
	}

	closed := c.conn.NotifyClose(make(chan *amqp.Error, 1))
	log.Printf("order worker ready, consuming from queue %q", queueName)

	for {
		select {
		case <-ctx.Done():
			// Stop new deliveries, then drain what the broker already sent so
			// those messages are redelivered rather than lost.
			if err := c.channel.Cancel(consumerTag, false); err != nil {
				log.Printf("cancel consumer: %v", err)
			}
			for msg := range deliveries {
				if err := msg.Nack(false, true); err != nil {
					log.Printf("nack during shutdown: %v", err)
				}
			}
			return nil

		case amqpErr := <-closed:
			if amqpErr == nil {
				return ErrConnectionClosed
			}
			return fmt.Errorf("%w: %w", ErrConnectionClosed, amqpErr)

		case msg, ok := <-deliveries:
			if !ok {
				return ErrConnectionClosed
			}
			c.handle(ctx, msg)
		}
	}
}

// handle processes one delivery and acknowledges it according to whether the
// failure is retryable.
func (c *Consumer) handle(ctx context.Context, msg amqp.Delivery) {
	event, err := decode(msg.Body)
	if err != nil {
		// A message we cannot parse will never parse. Discard it.
		log.Printf("discarding unprocessable message: %v", err)
		reject(msg, false)
		return
	}

	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.dbTimeout)
	defer cancel()

	status, err := c.store.Persist(dbCtx, event)
	switch {
	case err == nil:
		log.Printf("order %s persisted with status %s", event.OrderID, status)
		acknowledge(msg)

	case errors.Is(err, repository.ErrAlreadyPersisted):
		// At-least-once delivery: the order is already stored, so this is a
		// successful outcome, not an error.
		log.Printf("order %s already persisted, acknowledging redelivery", event.OrderID)
		acknowledge(msg)

	case errors.Is(err, repository.ErrPermanent):
		// Retrying cannot help. Drop the message instead of requeueing it
		// forever, which would block every order behind it.
		log.Printf("discarding order %s, permanent failure: %v", event.OrderID, err)
		reject(msg, false)

	default:
		// Transient failure such as a database outage. Requeue after a short
		// delay so redelivery does not become a hot loop.
		log.Printf("transient failure for order %s, requeueing: %v", event.OrderID, err)
		time.Sleep(retryDelay)
		reject(msg, true)
	}
}

func decode(body []byte) (models.OrderEvent, error) {
	var event models.OrderEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return event, fmt.Errorf("unmarshal order event: %w", err)
	}
	if err := event.Validate(); err != nil {
		return event, err
	}
	return event, nil
}

func acknowledge(msg amqp.Delivery) {
	if err := msg.Ack(false); err != nil {
		log.Printf("ack message: %v", err)
	}
}

func reject(msg amqp.Delivery, requeue bool) {
	if err := msg.Nack(false, requeue); err != nil {
		log.Printf("nack message: %v", err)
	}
}
