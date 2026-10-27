// Package publisher publishes order events to RabbitMQ with publisher
// confirms, so the purchase service only reports success once the broker has
// durably accepted the message.
package publisher

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/common/rabbitmq"
	"github.com/streadway/amqp"
)

// confirmTimeout bounds how long a publish waits for the broker's confirmation.
const confirmTimeout = 5 * time.Second

// ErrNotConfirmed reports that the broker did not acknowledge the publish, so
// the caller must assume the message was lost and compensate.
var ErrNotConfirmed = errors.New("publish not confirmed by broker")

// RabbitMQ publishes order events.
//
// An AMQP channel is not safe for concurrent use, and publisher confirms arrive
// on a single stream that must be paired with the publish that produced them.
// mu therefore serialises publish-and-wait so concurrent HTTP handlers cannot
// interleave their publishes or read each other's confirmations.
type RabbitMQ struct {
	mu       sync.Mutex
	conn     *amqp.Connection
	channel  *amqp.Channel
	confirms chan amqp.Confirmation
}

// NewRabbitMQ dials the broker, declares the topology and enables confirms.
func NewRabbitMQ(url string) (*RabbitMQ, error) {
	conn, err := rabbitmq.Dial(url)
	if err != nil {
		return nil, err
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	confirms := channel.NotifyPublish(make(chan amqp.Confirmation, 1))

	if err := rabbitmq.DeclareTopology(channel); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	log.Println("connected to RabbitMQ with publisher confirms")
	return &RabbitMQ{conn: conn, channel: channel, confirms: confirms}, nil
}

// Close releases the broker resources.
func (r *RabbitMQ) Close() {
	if r.channel != nil {
		_ = r.channel.Close()
	}
	if r.conn != nil {
		_ = r.conn.Close()
	}
}

// PublishOrderCreated publishes an order.created event and waits for the
// broker's confirmation. It returns an error unless the broker acknowledged the
// message, so a caller that has already reserved stock knows to release it.
func (r *RabbitMQ) PublishOrderCreated(event models.OrderEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal order event: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	msg := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Timestamp:    event.Timestamp,
		MessageId:    event.OrderID.String(),
	}
	if err := r.channel.Publish(rabbitmq.Exchange, rabbitmq.RoutingKeyOrderCreated, false, false, msg); err != nil {
		return fmt.Errorf("publish order %s: %w", event.OrderID, err)
	}

	select {
	case confirm, ok := <-r.confirms:
		if !ok {
			return fmt.Errorf("%w: channel closed", ErrNotConfirmed)
		}
		if !confirm.Ack {
			return fmt.Errorf("%w: broker returned nack for order %s", ErrNotConfirmed, event.OrderID)
		}
	case <-time.After(confirmTimeout):
		return fmt.Errorf("%w: timed out after %s", ErrNotConfirmed, confirmTimeout)
	}

	log.Printf("published order.created for order %s", event.OrderID)
	return nil
}
