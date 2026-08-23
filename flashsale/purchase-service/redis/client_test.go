package redis_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

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

func TestGetStockUnknownProduct(t *testing.T) {
	c := newTestClient(t)

	_, err := c.GetStock(context.Background(), uuid.New())
	if !errors.Is(err, redis.ErrProductUnknown) {
		t.Fatalf("GetStock() error = %v, want ErrProductUnknown", err)
	}
}

func TestInitializeAndGetStock(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	product := uuid.New()

	if err := c.InitializeStock(ctx, product, 42); err != nil {
		t.Fatalf("InitializeStock() error = %v", err)
	}
	got, err := c.GetStock(ctx, product)
	if err != nil {
		t.Fatalf("GetStock() error = %v", err)
	}
	if got != 42 {
		t.Errorf("GetStock() = %d, want 42", got)
	}
}

func TestDecrementStock(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	product := uuid.New()

	if err := c.InitializeStock(ctx, product, 5); err != nil {
		t.Fatalf("InitializeStock() error = %v", err)
	}

	remaining, err := c.DecrementStock(ctx, product, 3)
	if err != nil {
		t.Fatalf("DecrementStock() error = %v", err)
	}
	if remaining != 2 {
		t.Errorf("remaining = %d, want 2", remaining)
	}

	// Asking for more than is left must be refused outright, never partially
	// filled and never taking the counter negative.
	if _, err := c.DecrementStock(ctx, product, 3); !errors.Is(err, redis.ErrOutOfStock) {
		t.Fatalf("DecrementStock() beyond the remaining stock: error = %v, want ErrOutOfStock", err)
	}
	if left, _ := c.GetStock(ctx, product); left != 2 {
		t.Errorf("stock = %d after a refused decrement, want 2", left)
	}
}

func TestDecrementUnknownProduct(t *testing.T) {
	c := newTestClient(t)

	_, err := c.DecrementStock(context.Background(), uuid.New(), 1)
	if !errors.Is(err, redis.ErrProductUnknown) {
		t.Fatalf("DecrementStock() error = %v, want ErrProductUnknown", err)
	}
}

func TestIncrementStock(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	product := uuid.New()

	if err := c.InitializeStock(ctx, product, 1); err != nil {
		t.Fatalf("InitializeStock() error = %v", err)
	}
	if err := c.IncrementStock(ctx, product, 4); err != nil {
		t.Fatalf("IncrementStock() error = %v", err)
	}
	got, err := c.GetStock(ctx, product)
	if err != nil {
		t.Fatalf("GetStock() error = %v", err)
	}
	if got != 5 {
		t.Errorf("GetStock() = %d, want 5", got)
	}
}

// Only the first claim on a (buyer, product) pair may succeed: that is what
// makes a resubmitted order idempotent.
func TestClaimPurchaseIsExclusive(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	user, product := uuid.New(), uuid.New()

	first, err := c.ClaimPurchase(ctx, user, product, time.Minute)
	if err != nil || !first {
		t.Fatalf("first ClaimPurchase() = (%v, %v), want (true, nil)", first, err)
	}

	second, err := c.ClaimPurchase(ctx, user, product, time.Minute)
	if err != nil {
		t.Fatalf("second ClaimPurchase() error = %v", err)
	}
	if second {
		t.Error("second ClaimPurchase() = true, want false")
	}

	// Releasing must let the buyer retry immediately.
	if err := c.ReleasePurchaseClaim(ctx, user, product); err != nil {
		t.Fatalf("ReleasePurchaseClaim() error = %v", err)
	}
	third, err := c.ClaimPurchase(ctx, user, product, time.Minute)
	if err != nil || !third {
		t.Fatalf("ClaimPurchase() after release = (%v, %v), want (true, nil)", third, err)
	}
	t.Cleanup(func() { c.ReleasePurchaseClaim(context.Background(), user, product) })
}

func TestClaimPurchaseExpires(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	user, product := uuid.New(), uuid.New()

	if _, err := c.ClaimPurchase(ctx, user, product, 300*time.Millisecond); err != nil {
		t.Fatalf("ClaimPurchase() error = %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	again, err := c.ClaimPurchase(ctx, user, product, time.Minute)
	if err != nil || !again {
		t.Fatalf("ClaimPurchase() after the TTL = (%v, %v), want (true, nil)", again, err)
	}
	t.Cleanup(func() { c.ReleasePurchaseClaim(context.Background(), user, product) })
}

// The flash sale's core guarantee: however many buyers arrive at once, exactly
// the available units are sold. This runs against a real Redis because the
// atomicity comes from the server executing the Lua script, not from Go.
func TestDecrementStockNeverOversells(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	product := uuid.New()

	const (
		available = 50
		buyers    = 300
	)
	if err := c.InitializeStock(ctx, product, available); err != nil {
		t.Fatalf("InitializeStock() error = %v", err)
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		sold   int
		errsMu sync.Mutex
		errs   []error
	)
	start := make(chan struct{})

	for i := 0; i < buyers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := c.DecrementStock(ctx, product, 1)
			switch {
			case err == nil:
				mu.Lock()
				sold++
				mu.Unlock()
			case errors.Is(err, redis.ErrOutOfStock):
				// Expected once the stock runs out.
			default:
				errsMu.Lock()
				errs = append(errs, err)
				errsMu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors during concurrent decrements: %v", errs[0])
	}
	if sold != available {
		t.Errorf("sold %d units, want exactly %d", sold, available)
	}

	remaining, err := c.GetStock(ctx, product)
	if err != nil {
		t.Fatalf("GetStock() error = %v", err)
	}
	if remaining != 0 {
		t.Errorf("remaining stock = %d, want 0 (never negative, never left over)", remaining)
	}
}
