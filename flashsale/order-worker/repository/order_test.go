package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/flashsale/common/models"
	"github.com/flashsale/order-worker/repository"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// openTestDB connects to the database named by POSTGRES_URL. These tests turn
// on real transactional behaviour and constraint violations, so they need a
// real server rather than a mock.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		t.Skip("POSTGRES_URL is not set; skipping database integration tests")
	}

	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Skipf("database at POSTGRES_URL is unreachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertUser(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	email := "worker-test-" + uuid.NewString() + "@example.com"
	if err := db.QueryRow(
		`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', 'user') RETURNING id`,
		email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = $1`, id) })
	return id
}

func insertProduct(t *testing.T, db *sql.DB, stock int) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	name := "worker-test-product-" + uuid.NewString()
	if err := db.QueryRow(
		`INSERT INTO products (name, price, stock) VALUES ($1, 100, $2) RETURNING id`,
		name, stock).Scan(&id); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM products WHERE id = $1`, id) })
	return id
}

func stockOf(t *testing.T, db *sql.DB, productID uuid.UUID) int {
	t.Helper()

	var stock int
	if err := db.QueryRow(`SELECT stock FROM products WHERE id = $1`, productID).Scan(&stock); err != nil {
		t.Fatalf("read stock: %v", err)
	}
	return stock
}

// newEvent builds a valid event and schedules the order's removal.
func newEvent(t *testing.T, db *sql.DB, userID, productID uuid.UUID, qty int) models.OrderEvent {
	t.Helper()

	event := models.OrderEvent{
		OrderID:   uuid.New(),
		UserID:    userID,
		ProductID: productID,
		Qty:       qty,
		Timestamp: time.Now().UTC(),
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM orders WHERE id = $1`, event.OrderID) })
	return event
}

func TestPersistStoresOrderAndDeductsStock(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()

	user := insertUser(t, db)
	product := insertProduct(t, db, 10)
	event := newEvent(t, db, user, product, 3)

	status, err := repo.Persist(ctx, event)
	if err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if status != models.OrderStatusSuccess {
		t.Errorf("status = %s, want %s", status, models.OrderStatusSuccess)
	}
	if got := stockOf(t, db, product); got != 7 {
		t.Errorf("stock = %d, want 7", got)
	}

	stored, err := repo.FindStatus(ctx, event.OrderID)
	if err != nil {
		t.Fatalf("FindStatus() error = %v", err)
	}
	if stored != models.OrderStatusSuccess {
		t.Errorf("stored status = %s, want %s", stored, models.OrderStatusSuccess)
	}
}

// RabbitMQ delivers at least once, so the same event will arrive again after a
// restart or a lost acknowledgement. Redelivery must be a no-op, not a second
// order and not a second stock deduction.
func TestPersistIsIdempotentAcrossRedelivery(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()

	user := insertUser(t, db)
	product := insertProduct(t, db, 10)
	event := newEvent(t, db, user, product, 2)

	if _, err := repo.Persist(ctx, event); err != nil {
		t.Fatalf("first Persist() error = %v", err)
	}

	_, err := repo.Persist(ctx, event)
	if !errors.Is(err, repository.ErrAlreadyPersisted) {
		t.Fatalf("redelivery: error = %v, want ErrAlreadyPersisted", err)
	}
	if got := stockOf(t, db, product); got != 8 {
		t.Errorf("stock = %d, want 8: redelivery deducted stock twice", got)
	}
}

// An event naming a user or product that does not exist can never be stored, so
// it must be reported as permanent. Treating it as retryable is what turned a
// single bad message into an endless requeue loop.
func TestPersistReportsMissingReferencesAsPermanent(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()

	user := insertUser(t, db)
	product := insertProduct(t, db, 10)

	tests := []struct {
		name  string
		event models.OrderEvent
	}{
		{"unknown user", newEvent(t, db, uuid.New(), product, 1)},
		{"unknown product", newEvent(t, db, user, uuid.New(), 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.Persist(ctx, tt.event)
			if !errors.Is(err, repository.ErrPermanent) {
				t.Fatalf("Persist() error = %v, want ErrPermanent", err)
			}
		})
	}
}

func TestPersistRejectsInvalidEvent(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)

	tests := []struct {
		name  string
		event models.OrderEvent
	}{
		{"empty event", models.OrderEvent{}},
		{"zero qty", models.OrderEvent{
			OrderID: uuid.New(), UserID: uuid.New(), ProductID: uuid.New(), Qty: 0,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.Persist(context.Background(), tt.event)
			if !errors.Is(err, repository.ErrPermanent) {
				t.Fatalf("Persist() error = %v, want ErrPermanent", err)
			}
		})
	}
}

// When the durable stock cannot cover the order the order is recorded as FAILED
// rather than reported as successful. Silently ignoring the shortfall used to
// leave an order marked SUCCESS with no stock deducted behind it.
func TestPersistRecordsFailureWhenStockIsShort(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()

	user := insertUser(t, db)
	product := insertProduct(t, db, 2)
	event := newEvent(t, db, user, product, 5)

	status, err := repo.Persist(ctx, event)
	if err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if status != models.OrderStatusFailed {
		t.Fatalf("status = %s, want %s", status, models.OrderStatusFailed)
	}

	// The stock floor must hold: the deduction must not have happened at all.
	if got := stockOf(t, db, product); got != 2 {
		t.Errorf("stock = %d, want 2: stock was deducted for a failed order", got)
	}

	stored, err := repo.FindStatus(ctx, event.OrderID)
	if err != nil {
		t.Fatalf("FindStatus() error = %v: the failed order was not recorded", err)
	}
	if stored != models.OrderStatusFailed {
		t.Errorf("stored status = %s, want %s", stored, models.OrderStatusFailed)
	}
}

// Persisting an order that exactly empties the stock must succeed: the boundary
// belongs to the buyer, not to the guard.
func TestPersistAllowsExactlyRemainingStock(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)

	user := insertUser(t, db)
	product := insertProduct(t, db, 3)
	event := newEvent(t, db, user, product, 3)

	status, err := repo.Persist(context.Background(), event)
	if err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if status != models.OrderStatusSuccess {
		t.Errorf("status = %s, want %s", status, models.OrderStatusSuccess)
	}
	if got := stockOf(t, db, product); got != 0 {
		t.Errorf("stock = %d, want 0", got)
	}
}
