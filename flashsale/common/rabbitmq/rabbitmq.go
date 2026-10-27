// Package rabbitmq holds the broker topology shared by the order publisher and
// the order consumer. Defining it once keeps both sides bound to the same
// exchange, queue and routing key.
package rabbitmq

import (
	"fmt"
	"log"
	"time"

	"github.com/streadway/amqp"
)

// Topology names. Both the purchase service and the order worker declare them,
// so whichever starts first creates them.
const (
	Exchange               = "flashsale"
	Queue                  = "orders.queue"
	RoutingKeyOrderCreated = "order.created"
)

const (
	connectAttempts = 5
	connectBackoff  = 2 * time.Second
)

// Dial connects to the broker, retrying to tolerate a broker that is still
// starting alongside the service.
func Dial(url string) (*amqp.Connection, error) {
	var err error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		var conn *amqp.Connection
		if conn, err = amqp.Dial(url); err == nil {
			return conn, nil
		}
		log.Printf("rabbitmq not ready (attempt %d/%d): %v", attempt, connectAttempts, err)
		time.Sleep(connectBackoff)
	}
	return nil, fmt.Errorf("connect to rabbitmq after %d attempts: %w", connectAttempts, err)
}

// DeclareTopology declares the durable exchange and queue and binds them. It is
// idempotent, so every service can call it on startup.
func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", Exchange, err)
	}
	if _, err := ch.QueueDeclare(Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", Queue, err)
	}
	if err := ch.QueueBind(Queue, RoutingKeyOrderCreated, Exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue %q: %w", Queue, err)
	}
	return nil
}
