package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/purchase-service/service"
	"github.com/google/uuid"
)

// fakeStock is an in-memory StockStore that mirrors the atomicity the Redis
// implementation provides.
type fakeStock struct {
	mu       sync.Mutex
	stock    map[uuid.UUID]int
	claims   map[string]bool
	released int
	// claimErr and decrementErr force the corresponding operation to fail.
	claimErr     error
	decrementErr error
}

func newFakeStock() *fakeStock {
	return &fakeStock{stock: map[uuid.UUID]int{}, claims: map[string]bool{}}
}

func claimKey(userID, productID uuid.UUID) string { return userID.String() + ":" + productID.String() }

func (f *fakeStock) ClaimPurchase(_ context.Context, userID, productID uuid.UUID, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return false, f.claimErr
	}
	key := claimKey(userID, productID)
	if f.claims[key] {
		return false, nil
	}
	f.claims[key] = true
	return true, nil
}

func (f *fakeStock) ReleasePurchaseClaim(_ context.Context, userID, productID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released++
	delete(f.claims, claimKey(userID, productID))
	return nil
}

func (f *fakeStock) DecrementStock(_ context.Context, productID uuid.UUID, qty int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.decrementErr != nil {
		return 0, f.decrementErr
	}
	remaining, known := f.stock[productID]
	if !known {
		return 0, service.ErrProductUnknown
	}
	if remaining < qty {
		return 0, service.ErrOutOfStock
	}
	f.stock[productID] = remaining - qty
	return int64(remaining - qty), nil
}

func (f *fakeStock) IncrementStock(_ context.Context, productID uuid.UUID, qty int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stock[productID] += qty
	return nil
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

func (f *fakePublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.published)
}

func TestProcessReservesStockAndPublishes(t *testing.T) {
	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = 10
	pub := &fakePublisher{}
	svc := service.NewPurchase(stock, pub, time.Minute)

	user := uuid.New()
	orderID, err := svc.Process(context.Background(), user, product, 3)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if orderID == uuid.Nil {
		t.Fatal("Process() returned the nil order ID")
	}
	if got := stock.remaining(product); got != 7 {
		t.Errorf("remaining stock = %d, want 7", got)
	}

	if pub.count() != 1 {
		t.Fatalf("published %d events, want 1", pub.count())
	}
	event := pub.published[0]
	if event.OrderID != orderID || event.UserID != user || event.ProductID != product || event.Qty != 3 {
		t.Errorf("event = %+v, want order %s for user %s, product %s, qty 3",
			event, orderID, user, product)
	}
	if err := event.Validate(); err != nil {
		t.Errorf("published event is not valid: %v", err)
	}
}

// The idempotency claim is what stops a double-submitted order being placed
// twice, and it must not consume stock the second time.
func TestProcessRejectsDuplicatePurchase(t *testing.T) {
	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = 10
	pub := &fakePublisher{}
	svc := service.NewPurchase(stock, pub, time.Minute)

	user := uuid.New()
	if _, err := svc.Process(context.Background(), user, product, 1); err != nil {
		t.Fatalf("first Process() error = %v", err)
	}

	_, err := svc.Process(context.Background(), user, product, 1)
	if !errors.Is(err, service.ErrDuplicatePurchase) {
		t.Fatalf("second Process() error = %v, want ErrDuplicatePurchase", err)
	}
	if got := stock.remaining(product); got != 9 {
		t.Errorf("remaining stock = %d, want 9: the duplicate consumed stock", got)
	}
	if pub.count() != 1 {
		t.Errorf("published %d events, want 1", pub.count())
	}
}

// A refused purchase must release the claim, otherwise a buyer who happened to
// hit a sold-out moment is locked out for the whole idempotency window.
func TestProcessReleasesClaimWhenStockIsRefused(t *testing.T) {
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
			svc := service.NewPurchase(stock, &fakePublisher{}, time.Minute)

			_, err := svc.Process(context.Background(), uuid.New(), product, 5)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Process() error = %v, want %v", err, tt.wantErr)
			}
			if stock.heldClaims() != 0 {
				t.Error("the purchase claim was not released after a refused purchase")
			}
		})
	}
}

// This is the inventory-loss bug: stock is reserved before the event is
// published, so a publish failure must give the units back. Otherwise they are
// gone from the sale and no order will ever consume them.
func TestProcessReturnsStockWhenPublishFails(t *testing.T) {
	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = 10
	pub := &fakePublisher{err: errors.New("broker unavailable")}
	svc := service.NewPurchase(stock, pub, time.Minute)

	user := uuid.New()
	_, err := svc.Process(context.Background(), user, product, 4)
	if err == nil {
		t.Fatal("Process() = nil, want an error when publishing fails")
	}

	if got := stock.remaining(product); got != 10 {
		t.Errorf("remaining stock = %d, want 10: reserved units were not returned", got)
	}
	if stock.heldClaims() != 0 {
		t.Error("the purchase claim was not released after a failed publish")
	}

	// Having compensated, the buyer must be able to retry straight away.
	pub.err = nil
	if _, err := svc.Process(context.Background(), user, product, 4); err != nil {
		t.Fatalf("retry after a failed publish: error = %v", err)
	}
}

func TestProcessRejectsNonPositiveQty(t *testing.T) {
	stock := newFakeStock()
	product := uuid.New()
	stock.stock[product] = 10
	svc := service.NewPurchase(stock, &fakePublisher{}, time.Minute)

	for _, qty := range []int{0, -1} {
		if _, err := svc.Process(context.Background(), uuid.New(), product, qty); err == nil {
			t.Errorf("Process() with qty %d = nil, want an error", qty)
		}
	}
	if got := stock.remaining(product); got != 10 {
		t.Errorf("remaining stock = %d, want 10", got)
	}
}

func TestProcessReportsClaimFailure(t *testing.T) {
	stock := newFakeStock()
	stock.claimErr = errors.New("connection refused")
	svc := service.NewPurchase(stock, &fakePublisher{}, time.Minute)

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
	svc := service.NewPurchase(stock, pub, time.Minute)

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
