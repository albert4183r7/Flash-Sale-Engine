package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/order-worker/consumer"
	"github.com/flashsale/order-worker/repository"
	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

const (
	exchangeName = "flashsale"
	queueName    = "orders.queue"
	routingKey   = "order.created"
)

// recordingStore is an OrderStore whose outcome per order is scripted, so the
// consumer's acknowledgement decisions can be observed directly.
type recordingStore struct {
	mu sync.Mutex
	// outcomes maps an order ID to the error Persist should return.
	outcomes map[uuid.UUID]error
	// attempts counts Persist calls per order, which is how a requeue loop
	// becomes visible.
	attempts map[uuid.UUID]int
	done     chan uuid.UUID
}

func newRecordingStore() *recordingStore {
	return &recordingStore{
		outcomes: map[uuid.UUID]error{},
		attempts: map[uuid.UUID]int{},
		done:     make(chan uuid.UUID, 64),
	}
}

func (s *recordingStore) Persist(_ context.Context, event models.OrderEvent) (models.OrderStatus, error) {
	s.mu.Lock()
	s.attempts[event.OrderID]++
	err := s.outcomes[event.OrderID]
	s.mu.Unlock()

	select {
	case s.done <- event.OrderID:
	default:
	}

	if err != nil {
		return "", err
	}
	return models.OrderStatusSuccess, nil
}

func (s *recordingStore) attemptsFor(id uuid.UUID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts[id]
}

// dialTestBroker connects to the broker named by RABBITMQ_URL and gives the
// test a clean queue to work with.
func dialTestBroker(t *testing.T) (*amqp.Channel, string) {
	t.Helper()

	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL is not set; skipping broker integration tests")
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Skipf("broker at RABBITMQ_URL is unreachable: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	t.Cleanup(func() { ch.Close() })

	if err := ch.ExchangeDeclare(exchangeName, "direct", true, false, false, false, nil); err != nil {
		t.Fatalf("declare exchange: %v", err)
	}
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		t.Fatalf("declare queue: %v", err)
	}
	if err := ch.QueueBind(queueName, routingKey, exchangeName, false, nil); err != nil {
		t.Fatalf("bind queue: %v", err)
	}
	// Start from a known-empty queue so leftovers cannot affect the assertions.
	if _, err := ch.QueuePurge(queueName, false); err != nil {
		t.Fatalf("purge queue: %v", err)
	}
	t.Cleanup(func() { ch.QueuePurge(queueName, false) })

	return ch, url
}

func publish(t *testing.T, ch *amqp.Channel, body []byte) {
	t.Helper()

	err := ch.Publish(exchangeName, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func publishEvent(t *testing.T, ch *amqp.Channel, event models.OrderEvent) {
	t.Helper()

	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	publish(t, ch, body)
}

func newEvent() models.OrderEvent {
	return models.OrderEvent{
		OrderID:   uuid.New(),
		UserID:    uuid.New(),
		ProductID: uuid.New(),
		Qty:       1,
		Timestamp: time.Now().UTC(),
	}
}

// runConsumer starts the consumer and stops it when the test ends.
func runConsumer(t *testing.T, url string, store consumer.OrderStore) {
	t.Helper()

	c, err := consumer.New(url, store, 5*time.Second)
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		if err := c.Run(ctx); err != nil {
			t.Errorf("Run() error = %v", err)
		}
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			t.Error("consumer did not shut down within 10s")
		}
		c.Close()
	})
}

// queueDepth reports how many messages are waiting, which is how a blocked
// queue is detected.
func queueDepth(t *testing.T, ch *amqp.Channel) int {
	t.Helper()

	q, err := ch.QueueDeclarePassive(queueName, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("inspect queue: %v", err)
	}
	return q.Messages
}

func TestConsumerPersistsOrder(t *testing.T) {
	ch, url := dialTestBroker(t)
	store := newRecordingStore()
	runConsumer(t, url, store)

	event := newEvent()
	publishEvent(t, ch, event)

	select {
	case got := <-store.done:
		if got != event.OrderID {
			t.Fatalf("processed order %s, want %s", got, event.OrderID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the consumer did not process the message within 10s")
	}
}

// This is the regression test for the failure that wedged the whole pipeline: a
// message that can never succeed used to be requeued forever, spinning at
// thousands of attempts per second and starving every order behind it.
//
// The poison message must be discarded after one attempt, and the good message
// published after it must still be processed.
func TestConsumerDiscardsPermanentFailureAndKeepsGoing(t *testing.T) {
	ch, url := dialTestBroker(t)
	store := newRecordingStore()

	poison := newEvent()
	store.outcomes[poison.OrderID] = errors.Join(repository.ErrPermanent,
		errors.New("orders_user_id_fkey violation"))

	runConsumer(t, url, store)

	publishEvent(t, ch, poison)
	good := newEvent()
	publishEvent(t, ch, good)

	// The good message must arrive, which it cannot do if the poison one is
	// blocking the queue.
	deadline := time.After(15 * time.Second)
	for {
		select {
		case id := <-store.done:
			if id == good.OrderID {
				goto processed
			}
		case <-deadline:
			t.Fatalf("the good message was never processed; poison attempts = %d",
				store.attemptsFor(poison.OrderID))
		}
	}

processed:
	if got := store.attemptsFor(poison.OrderID); got != 1 {
		t.Errorf("the poison message was attempted %d times, want exactly 1", got)
	}

	// Give the broker a moment to settle, then confirm nothing is left behind.
	time.Sleep(500 * time.Millisecond)
	if depth := queueDepth(t, ch); depth != 0 {
		t.Errorf("queue still holds %d message(s), want 0", depth)
	}
}

// A body that is not a valid order event can never become one, so it must be
// dropped rather than retried.
func TestConsumerDiscardsUnprocessableMessages(t *testing.T) {
	ch, url := dialTestBroker(t)
	store := newRecordingStore()
	runConsumer(t, url, store)

	publish(t, ch, []byte("this is not json"))
	// An event that parses but carries no usable identifiers.
	publish(t, ch, []byte(`{"qty":0}`))

	good := newEvent()
	publishEvent(t, ch, good)

	select {
	case id := <-store.done:
		if id != good.OrderID {
			t.Fatalf("processed order %s, want %s", id, good.OrderID)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the good message was never processed; malformed messages blocked the queue")
	}

	time.Sleep(500 * time.Millisecond)
	if depth := queueDepth(t, ch); depth != 0 {
		t.Errorf("queue still holds %d message(s), want 0", depth)
	}
}

// A redelivered order is a success, not an error, so it must be acknowledged
// and removed from the queue.
func TestConsumerAcknowledgesRedelivery(t *testing.T) {
	ch, url := dialTestBroker(t)
	store := newRecordingStore()

	event := newEvent()
	store.outcomes[event.OrderID] = repository.ErrAlreadyPersisted

	runConsumer(t, url, store)
	publishEvent(t, ch, event)

	select {
	case <-store.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the consumer did not process the redelivery within 10s")
	}

	time.Sleep(500 * time.Millisecond)
	if got := store.attemptsFor(event.OrderID); got != 1 {
		t.Errorf("the redelivery was attempted %d times, want 1", got)
	}
	if depth := queueDepth(t, ch); depth != 0 {
		t.Errorf("queue still holds %d message(s), want 0", depth)
	}
}

// A transient failure must be retried, but slowly enough that redelivery does
// not become a hot loop.
func TestConsumerRequeuesTransientFailure(t *testing.T) {
	ch, url := dialTestBroker(t)
	store := newRecordingStore()

	event := newEvent()
	store.outcomes[event.OrderID] = errors.New("connection refused")

	runConsumer(t, url, store)
	publishEvent(t, ch, event)

	// Wait for the first attempt, then let one retry delay elapse.
	select {
	case <-store.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the consumer did not attempt the message within 10s")
	}
	time.Sleep(3 * time.Second)

	attempts := store.attemptsFor(event.OrderID)
	if attempts < 2 {
		t.Errorf("the message was attempted %d times, want it to be retried", attempts)
	}
	// Without the delay this used to reach thousands of attempts per second.
	if attempts > 10 {
		t.Errorf("the message was attempted %d times in ~3s, which is a hot retry loop", attempts)
	}
}
