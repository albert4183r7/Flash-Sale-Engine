package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/purchase-service/redis"
	"github.com/flashsale/purchase-service/service"
	"github.com/google/uuid"
)

// fakeStock is an in-memory StockStore that mirrors the atomicity the Redis
// scripts provide: every method runs under one lock.
type fakeStock struct {
	mu     sync.Mutex
	stock  map[uuid.UUID]int
	claims map[string]uuid.UUID
	outbox map[uuid.UUID]models.OrderEvent
	// reserveErr forces Reserve to fail.
	reserveErr error
}

func newFakeStock() *fakeStock {
	return &fakeStock{
		stock:  map[uuid.UUID]int{},
		claims: map[string]uuid.UUID{},
		outbox: map[uuid.UUID]models.OrderEvent{},
	}
}

func claimKey(userID, productID uuid.UUID) string { return userID.String() + ":" + productID.String() }

func (f *fakeStock) Reserve(_ context.Context, r redis.Reservation) (redis.Reserved, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reserveErr != nil {
		return redis.Reserved{}, f.reserveErr
	}
	e := r.Event
	if original, held := f.claims[claimKey(e.UserID, e.ProductID)]; held {
		return redis.Reserved{OrderID: original, Duplicate: true}, nil
	}
	remaining, known := f.stock[e.ProductID]
	if !known {
		return redis.Reserved{}, redis.ErrProductUnknown
	}
	if remaining < e.Qty {
		return redis.Reserved{}, redis.ErrOutOfStock
	}
	f.stock[e.ProductID] = remaining - e.Qty
	f.claims[claimKey(e.UserID, e.ProductID)] = e.OrderID
	f.outbox[e.OrderID] = e
	return redis.Reserved{OrderID: e.OrderID, Remaining: int64(remaining - e.Qty)}, nil
}

func (f *fakeStock) Release(_ context.Context, e models.OrderEvent) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.outbox[e.OrderID]; !ok {
		return false, nil
	}
	delete(f.outbox, e.OrderID)
	f.stock[e.ProductID] += e.Qty
	delete(f.claims, claimKey(e.UserID, e.ProductID))
	return true, nil
}

func (f *fakeStock) MarkPublished(_ context.Context, orderID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.outbox, orderID)
	return nil
}

func (f *fakeStock) StaleOutbox(_ context.Context, olderThan time.Duration) ([]models.OrderEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var stale []models.OrderEvent
	for _, e := range f.outbox {
		if time.Since(e.Timestamp) > olderThan {
			stale = append(stale, e)
		}
	}
	return stale, nil
}

func (f *fakeStock) remaining(productID uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stock[productID]
}

func (f *fakeStock) heldClaims() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.claims)
}

func (f *fakeStock) outboxSize() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.outbox)
}

// fakePublisher records published events and can be made to fail.
type fakePublisher struct {
	mu        sync.Mutex
	published []models.OrderEvent
	err       error
}

func (f *fakePublisher) PublishOrderCreated(event models.OrderEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, event)
	return nil
}

func (f *fakePublisher) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakePublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.published)
}

var ttls = service.TTLs{Idempotency: time.Minute, PendingOrder: time.Minute}

func TestProcessReservesStockAndPublishes(t *testing.T) {
	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = 10
	pub := &fakePublisher{}
	svc := service.NewPurchase(stock, pub, ttls)

	user := uuid.New()
	placed, err := svc.Process(context.Background(), user, product, 3)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if placed.OrderID == uuid.Nil || placed.Duplicate {
		t.Fatalf("Process() = %+v, want a new order", placed)
	}
	if got := stock.remaining(product); got != 7 {
		t.Errorf("remaining stock = %d, want 7", got)
	}

	if pub.count() != 1 {
		t.Fatalf("published %d events, want 1", pub.count())
	}
	event := pub.published[0]
	if event.OrderID != placed.OrderID || event.UserID != user || event.ProductID != product || event.Qty != 3 {
		t.Errorf("event = %+v, want order %s for user %s, product %s, qty 3",
			event, placed.OrderID, user, product)
	}
	if err := event.Validate(); err != nil {
		t.Errorf("published event is not valid: %v", err)
	}
	if stock.outboxSize() != 0 {
		t.Error("a confirmed event was left in the outbox")
	}
}

// A resubmission within the idempotency window must return the original order,
// without reserving stock or publishing a second time. This is what lets a
// client safely retry a purchase whose response it never received.
func TestProcessReturnsOriginalOrderForDuplicate(t *testing.T) {
	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = 10
	pub := &fakePublisher{}
	svc := service.NewPurchase(stock, pub, ttls)

	user := uuid.New()
	first, err := svc.Process(context.Background(), user, product, 1)
	if err != nil {
		t.Fatalf("first Process() error = %v", err)
	}

	second, err := svc.Process(context.Background(), user, product, 1)
	if err != nil {
		t.Fatalf("second Process() error = %v", err)
	}
	if !second.Duplicate || second.OrderID != first.OrderID {
		t.Fatalf("second Process() = %+v, want duplicate of order %s", second, first.OrderID)
	}
	if got := stock.remaining(product); got != 9 {
		t.Errorf("remaining stock = %d, want 9: the duplicate consumed stock", got)
	}
	if pub.count() != 1 {
		t.Errorf("published %d events, want 1", pub.count())
	}
}

// A refused purchase must not hold a claim, otherwise a buyer who happened to
// hit a sold-out moment is locked out for the whole idempotency window.
func TestProcessHoldsNoClaimWhenStockIsRefused(t *testing.T) {
	tests := []struct {
		name    string
		stockOf int
		known   bool
		wantErr error
	}{
		{"out of stock", 1, true, service.ErrOutOfStock},
		{"unknown product", 0, false, service.ErrProductUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stock := newFakeStock()
			product := uuid.New()
			if tt.known {
				stock.stock[product] = tt.stockOf
			}
			svc := service.NewPurchase(stock, &fakePublisher{}, ttls)

			_, err := svc.Process(context.Background(), uuid.New(), product, 5)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Process() error = %v, want %v", err, tt.wantErr)
			}
			if stock.heldClaims() != 0 {
				t.Error("a refused purchase left a claim behind")
			}
		})
	}
}

// Stock is reserved before the event is published, so a publish failure must
// give the units back. Otherwise they are gone from the sale and no order will
// ever consume them.
func TestProcessReleasesReservationWhenPublishFails(t *testing.T) {
	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = 10
	pub := &fakePublisher{err: errors.New("broker unavailable")}
	svc := service.NewPurchase(stock, pub, ttls)

	user := uuid.New()
	if _, err := svc.Process(context.Background(), user, product, 4); err == nil {
		t.Fatal("Process() = nil, want an error when publishing fails")
	}

	if got := stock.remaining(product); got != 10 {
		t.Errorf("remaining stock = %d, want 10: reserved units were not returned", got)
	}
	if stock.heldClaims() != 0 {
		t.Error("the purchase claim was not released after a failed publish")
	}
	if stock.outboxSize() != 0 {
		t.Error("a released order was left in the outbox")
	}

	// Having released the reservation, the buyer must be able to retry
	// straight away.
	pub.fail(nil)
	if _, err := svc.Process(context.Background(), user, product, 4); err != nil {
		t.Fatalf("retry after a failed publish: error = %v", err)
	}
}

// An event whose request died before confirming it must still be published.
// Without the relay, its stock would stay reserved by an order that never
// reaches the worker.
func TestRepublishStalePublishesInterruptedEvents(t *testing.T) {
	stock := newFakeStock()
	pub := &fakePublisher{}
	svc := service.NewPurchase(stock, pub, ttls)

	interrupted := models.OrderEvent{
		OrderID: uuid.New(), UserID: uuid.New(), ProductID: uuid.New(), Qty: 1,
		Timestamp: time.Now().Add(-time.Minute),
	}
	inFlight := models.OrderEvent{
		OrderID: uuid.New(), UserID: uuid.New(), ProductID: uuid.New(), Qty: 1,
		Timestamp: time.Now(),
	}
	stock.outbox[interrupted.OrderID] = interrupted
	stock.outbox[inFlight.OrderID] = inFlight

	published, err := svc.RepublishStale(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatalf("RepublishStale() error = %v", err)
	}
	if published != 1 || pub.count() != 1 || pub.published[0].OrderID != interrupted.OrderID {
		t.Fatalf("RepublishStale() published %d (%v), want only the interrupted order", published, pub.published)
	}

	// The fresh event belongs to a request that may still be waiting on the
	// broker, so it must be left alone.
	if _, still := stock.outbox[inFlight.OrderID]; !still {
		t.Error("an in-flight event was taken out of the outbox")
	}
	if _, still := stock.outbox[interrupted.OrderID]; still {
		t.Error("a republished event was left in the outbox")
	}
}

// While the broker is down nothing may leave the outbox, so the next run can
// still publish it.
func TestRepublishStaleKeepsEventsWhenBrokerFails(t *testing.T) {
	stock := newFakeStock()
	pub := &fakePublisher{err: errors.New("broker unavailable")}
	svc := service.NewPurchase(stock, pub, ttls)

	event := models.OrderEvent{
		OrderID: uuid.New(), UserID: uuid.New(), ProductID: uuid.New(), Qty: 1,
		Timestamp: time.Now().Add(-time.Minute),
	}
	stock.outbox[event.OrderID] = event

	if _, err := svc.RepublishStale(context.Background(), 30*time.Second); err == nil {
		t.Fatal("RepublishStale() = nil, want the broker error")
	}
	if stock.outboxSize() != 1 {
		t.Error("an unpublished event was dropped from the outbox")
	}
}

func TestProcessRejectsNonPositiveQty(t *testing.T) {
	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = 10
	svc := service.NewPurchase(stock, &fakePublisher{}, ttls)

	for _, qty := range []int{0, -1} {
		if _, err := svc.Process(context.Background(), uuid.New(), product, qty); err == nil {
			t.Errorf("Process() with qty %d = nil, want an error", qty)
		}
	}
	if got := stock.remaining(product); got != 10 {
		t.Errorf("remaining stock = %d, want 10", got)
	}
}

func TestProcessReportsStoreFailure(t *testing.T) {
	stock := newFakeStock()
	stock.reserveErr = errors.New("connection refused")
	svc := service.NewPurchase(stock, &fakePublisher{}, ttls)

	if _, err := svc.Process(context.Background(), uuid.New(), uuid.New(), 1); err == nil {
		t.Fatal("Process() = nil, want an error when the stock store is unavailable")
	}
}

// The whole point of the design is that concurrent buyers cannot oversell. Each
// buyer gets their own claim, so they all contend for the same stock counter.
func TestProcessDoesNotOversellUnderConcurrency(t *testing.T) {
	const (
		available = 20
		buyers    = 100
	)

	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = available
	pub := &fakePublisher{}
	svc := service.NewPurchase(stock, pub, ttls)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted int
	)
	start := make(chan struct{})

	for i := 0; i < buyers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := svc.Process(context.Background(), uuid.New(), product, 1); err == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if accepted != available {
		t.Errorf("accepted %d purchases, want exactly %d", accepted, available)
	}
	if got := stock.remaining(product); got != 0 {
		t.Errorf("remaining stock = %d, want 0", got)
	}
	if pub.count() != available {
		t.Errorf("published %d events, want %d", pub.count(), available)
	}
}
