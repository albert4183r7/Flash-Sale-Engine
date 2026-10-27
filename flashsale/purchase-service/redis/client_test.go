package redis_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/purchase-service/redis"
	"github.com/google/uuid"
)

// newTestClient connects to the Redis named by REDIS_ADDR. The stock logic
// lives in a Lua script executed by Redis, so only a real server can prove it
// is atomic.
func newTestClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR is not set; skipping Redis integration tests")
	}

	c := redis.NewClient(addr, os.Getenv("REDIS_PASSWORD"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		c.Close()
		t.Skipf("Redis at REDIS_ADDR is unreachable: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// newEvent builds an order event for a fresh buyer.
func newEvent(product uuid.UUID, qty int) models.OrderEvent {
	return models.OrderEvent{
		OrderID: uuid.New(), UserID: uuid.New(), ProductID: product, Qty: qty,
		Timestamp: time.Now().UTC(),
	}
}

// reserve places an order and removes it from the shared outbox afterwards, so
// it cannot leak into another test or a running relay.
func reserve(t *testing.T, c *redis.Client, e models.OrderEvent, claimTTL time.Duration) (redis.Reserved, error) {
	t.Helper()
	t.Cleanup(func() { c.MarkPublished(context.Background(), e.OrderID) })
	return c.Reserve(context.Background(), redis.Reservation{Event: e, ClaimTTL: claimTTL, PendingTTL: time.Minute})
}

func newProduct(t *testing.T, c *redis.Client, stock int) uuid.UUID {
	t.Helper()
	product := uuid.New()
	if _, err := c.InitializeStock(context.Background(), product, stock); err != nil {
		t.Fatalf("InitializeStock() error = %v", err)
	}
	return product
}

func stockOf(t *testing.T, c *redis.Client, product uuid.UUID) int64 {
	t.Helper()
	got, err := c.GetStock(context.Background(), product)
	if err != nil {
		t.Fatalf("GetStock() error = %v", err)
	}
	return got
}

func TestGetStockUnknownProduct(t *testing.T) {
	c := newTestClient(t)

	_, err := c.GetStock(context.Background(), uuid.New())
	if !errors.Is(err, redis.ErrProductUnknown) {
		t.Fatalf("GetStock() error = %v, want ErrProductUnknown", err)
	}
}

// Loading stock must never overwrite a live counter: it already accounts for
// orders still on their way to the database, and resetting it would oversell.
func TestInitializeStockKeepsLiveCounter(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	product := uuid.New()

	created, err := c.InitializeStock(ctx, product, 42)
	if err != nil || !created {
		t.Fatalf("first InitializeStock() = (%v, %v), want (true, nil)", created, err)
	}
	created, err = c.InitializeStock(ctx, product, 99)
	if err != nil || created {
		t.Fatalf("second InitializeStock() = (%v, %v), want (false, nil)", created, err)
	}
	if got := stockOf(t, c, product); got != 42 {
		t.Errorf("stock = %d, want 42: a live counter was overwritten", got)
	}
}

func TestIncrementStock(t *testing.T) {
	c := newTestClient(t)
	product := newProduct(t, c, 1)

	if err := c.IncrementStock(context.Background(), product, 4); err != nil {
		t.Fatalf("IncrementStock() error = %v", err)
	}
	if got := stockOf(t, c, product); got != 5 {
		t.Errorf("stock = %d, want 5", got)
	}
}

// A reservation writes the stock, the outbox and the pending copy together.
func TestReserveRecordsTheOrder(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	product := newProduct(t, c, 5)
	event := newEvent(product, 3)

	got, err := reserve(t, c, event, time.Minute)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if got.Duplicate || got.OrderID != event.OrderID || got.Remaining != 2 {
		t.Errorf("Reserve() = %+v, want new order %s with 2 remaining", got, event.OrderID)
	}
	if left := stockOf(t, c, product); left != 2 {
		t.Errorf("stock = %d, want 2", left)
	}

	pending, err := c.PendingOrder(ctx, event.OrderID)
	if err != nil {
		t.Fatalf("PendingOrder() error = %v", err)
	}
	if pending.OrderID != event.OrderID || pending.UserID != event.UserID || pending.Qty != 3 {
		t.Errorf("PendingOrder() = %+v, want %+v", pending, event)
	}

	stale, err := c.StaleOutbox(ctx, -time.Minute)
	if err != nil {
		t.Fatalf("StaleOutbox() error = %v", err)
	}
	if !containsOrder(stale, event.OrderID) {
		t.Error("the reserved order is not in the outbox")
	}
}

// A refused reservation must write nothing: no stock change and no claim, so
// the buyer can retry immediately.
func TestReserveRefusesWithoutSideEffects(t *testing.T) {
	c := newTestClient(t)
	product := newProduct(t, c, 2)
	event := newEvent(product, 3)

	if _, err := reserve(t, c, event, time.Minute); !errors.Is(err, redis.ErrOutOfStock) {
		t.Fatalf("Reserve() beyond the remaining stock: error = %v, want ErrOutOfStock", err)
	}
	if left := stockOf(t, c, product); left != 2 {
		t.Errorf("stock = %d after a refused reservation, want 2", left)
	}

	// The same buyer may immediately order an amount that fits.
	event.OrderID, event.Qty = uuid.New(), 2
	if got, err := reserve(t, c, event, time.Minute); err != nil || got.Duplicate {
		t.Fatalf("Reserve() after a refusal = (%+v, %v), want a new order", got, err)
	}
}

func TestReserveUnknownProduct(t *testing.T) {
	c := newTestClient(t)

	_, err := reserve(t, c, newEvent(uuid.New(), 1), time.Minute)
	if !errors.Is(err, redis.ErrProductUnknown) {
		t.Fatalf("Reserve() error = %v, want ErrProductUnknown", err)
	}
}

// Within the claim window a resubmission gets the original order back and
// reserves nothing.
func TestReserveReturnsOriginalOrderForDuplicate(t *testing.T) {
	c := newTestClient(t)
	product := newProduct(t, c, 10)
	first := newEvent(product, 1)

	if _, err := reserve(t, c, first, time.Minute); err != nil {
		t.Fatalf("first Reserve() error = %v", err)
	}

	retry := first
	retry.OrderID = uuid.New()
	got, err := reserve(t, c, retry, time.Minute)
	if err != nil {
		t.Fatalf("second Reserve() error = %v", err)
	}
	if !got.Duplicate || got.OrderID != first.OrderID {
		t.Fatalf("second Reserve() = %+v, want duplicate of %s", got, first.OrderID)
	}
	if left := stockOf(t, c, product); left != 9 {
		t.Errorf("stock = %d, want 9: the duplicate consumed stock", left)
	}
}

func TestReserveClaimExpires(t *testing.T) {
	c := newTestClient(t)
	product := newProduct(t, c, 10)
	first := newEvent(product, 1)

	if _, err := reserve(t, c, first, 300*time.Millisecond); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	again := first
	again.OrderID = uuid.New()
	if got, err := reserve(t, c, again, time.Minute); err != nil || got.Duplicate {
		t.Fatalf("Reserve() after the claim expired = (%+v, %v), want a new order", got, err)
	}
}

// Releasing undoes the reservation exactly once, however often it is called.
func TestReleaseUndoesReservationOnce(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	product := newProduct(t, c, 5)
	event := newEvent(product, 2)

	if _, err := reserve(t, c, event, time.Minute); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	for i, want := range []bool{true, false} {
		released, err := c.Release(ctx, event)
		if err != nil || released != want {
			t.Fatalf("Release() call %d = (%v, %v), want (%v, nil)", i+1, released, err, want)
		}
	}
	if left := stockOf(t, c, product); left != 5 {
		t.Errorf("stock = %d, want 5: the release was applied more or less than once", left)
	}
	if _, err := c.PendingOrder(ctx, event.OrderID); !errors.Is(err, redis.ErrNotPending) {
		t.Errorf("PendingOrder() after release: error = %v, want ErrNotPending", err)
	}

	// The claim is gone, so the buyer can order again at once.
	event.OrderID = uuid.New()
	if got, err := reserve(t, c, event, time.Minute); err != nil || got.Duplicate {
		t.Fatalf("Reserve() after release = (%+v, %v), want a new order", got, err)
	}
}

// Once published, an order belongs to the worker: a late release must not hand
// its stock back.
func TestReleaseIgnoresPublishedOrder(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	product := newProduct(t, c, 5)
	event := newEvent(product, 2)

	if _, err := reserve(t, c, event, time.Minute); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if err := c.MarkPublished(ctx, event.OrderID); err != nil {
		t.Fatalf("MarkPublished() error = %v", err)
	}

	released, err := c.Release(ctx, event)
	if err != nil || released {
		t.Fatalf("Release() = (%v, %v), want (false, nil)", released, err)
	}
	if left := stockOf(t, c, product); left != 3 {
		t.Errorf("stock = %d, want 3", left)
	}
}

func TestStaleOutboxSkipsFreshEvents(t *testing.T) {
	c := newTestClient(t)
	product := newProduct(t, c, 5)
	event := newEvent(product, 1)

	if _, err := reserve(t, c, event, time.Minute); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	stale, err := c.StaleOutbox(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("StaleOutbox() error = %v", err)
	}
	if containsOrder(stale, event.OrderID) {
		t.Error("a fresh event was reported as stale")
	}
}

func TestPendingOrderUnknown(t *testing.T) {
	c := newTestClient(t)

	_, err := c.PendingOrder(context.Background(), uuid.New())
	if !errors.Is(err, redis.ErrNotPending) {
		t.Fatalf("PendingOrder() error = %v, want ErrNotPending", err)
	}
}

func containsOrder(events []models.OrderEvent, orderID uuid.UUID) bool {
	for _, e := range events {
		if e.OrderID == orderID {
			return true
		}
	}
	return false
}

// The flash sale's core guarantee: however many buyers arrive at once, exactly
// the available units are sold. This runs against a real Redis because the
// atomicity comes from the server executing the Lua script, not from Go.
func TestReserveNeverOversells(t *testing.T) {
	c := newTestClient(t)

	const (
		available = 50
		buyers    = 300
	)
	product := newProduct(t, c, available)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		sold int
		errs []error
	)
	start := make(chan struct{})

	for i := 0; i < buyers; i++ {
		event := newEvent(product, 1)
		t.Cleanup(func() { c.MarkPublished(context.Background(), event.OrderID) })

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := c.Reserve(context.Background(), redis.Reservation{
				Event: event, ClaimTTL: time.Minute, PendingTTL: time.Minute,
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				sold++
			case errors.Is(err, redis.ErrOutOfStock):
				// Expected once the stock runs out.
			default:
				errs = append(errs, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors during concurrent reservations: %v", errs[0])
	}
	if sold != available {
		t.Errorf("sold %d units, want exactly %d", sold, available)
	}
	if left := stockOf(t, c, product); left != 0 {
		t.Errorf("remaining stock = %d, want 0 (never negative, never left over)", left)
	}
}
