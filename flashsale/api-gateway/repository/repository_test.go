package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/flashsale/api-gateway/repository"
	"github.com/flashsale/common/models"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// openTestDB connects to the database named by POSTGRES_URL. These tests
// exercise real SQL — transactions, constraints and row locking — which a mock
// cannot verify, so they are skipped when no database is configured.
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

// insertUser creates a throwaway account and removes it afterwards.
func insertUser(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	email := "test-" + uuid.NewString() + "@example.com"
	err := db.QueryRow(
		`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', 'user') RETURNING id`,
		email).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = $1`, id) })
	return id
}

// insertProduct creates a throwaway product with the given stock.
func insertProduct(t *testing.T, db *sql.DB, stock int) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	name := "test-product-" + uuid.NewString()
	err := db.QueryRow(
		`INSERT INTO products (name, price, stock) VALUES ($1, 100, $2) RETURNING id`,
		name, stock).Scan(&id)
	if err != nil {
		t.Fatalf("insert product: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM products WHERE id = $1`, id) })
	return id
}

// insertOrder creates an order in the given state.
func insertOrder(t *testing.T, db *sql.DB, userID, productID uuid.UUID, qty int, status models.OrderStatus) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.Exec(
		`INSERT INTO orders (id, user_id, product_id, qty, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())`,
		id, userID, productID, qty, status)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM orders WHERE id = $1`, id) })
	return id
}

func productStock(t *testing.T, db *sql.DB, productID uuid.UUID) int {
	t.Helper()

	var stock int
	if err := db.QueryRow(`SELECT stock FROM products WHERE id = $1`, productID).Scan(&stock); err != nil {
		t.Fatalf("read stock: %v", err)
	}
	return stock
}

func TestUserRepositoryCreateAndFind(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewUserRepository(db)
	ctx := context.Background()

	email := "test-" + uuid.NewString() + "@example.com"
	if err := repo.Create(ctx, email, "hashed", "user"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE email = $1`, email) })

	creds, err := repo.FindCredentialsByEmail(ctx, email)
	if err != nil {
		t.Fatalf("FindCredentialsByEmail() error = %v", err)
	}
	if creds.User.Email != email || creds.PasswordHash != "hashed" || creds.User.Role != "user" {
		t.Errorf("credentials = %+v, want email %q with role \"user\"", creds, email)
	}
}

// A duplicate signup must be reported as a duplicate, not as a generic failure,
// and a genuine failure must never be reported as a duplicate.
func TestUserRepositoryCreateRejectsDuplicateEmail(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewUserRepository(db)
	ctx := context.Background()

	email := "test-" + uuid.NewString() + "@example.com"
	if err := repo.Create(ctx, email, "hashed", "user"); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE email = $1`, email) })

	err := repo.Create(ctx, email, "hashed", "user")
	if !errors.Is(err, repository.ErrEmailTaken) {
		t.Fatalf("second Create() error = %v, want ErrEmailTaken", err)
	}
}

func TestUserRepositoryFindMissing(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewUserRepository(db)

	_, err := repo.FindCredentialsByEmail(context.Background(), "absent-"+uuid.NewString()+"@example.com")
	if !errors.Is(err, repository.ErrUserNotFound) {
		t.Fatalf("FindCredentialsByEmail() error = %v, want ErrUserNotFound", err)
	}
}

// The ownership filter is enforced in SQL. This is the regression test for the
// vulnerability where any authenticated user could read any order by ID.
func TestOrderRepositoryFindIsScopedToOwner(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()

	owner := insertUser(t, db)
	stranger := insertUser(t, db)
	product := insertProduct(t, db, 10)
	orderID := insertOrder(t, db, owner, product, 2, models.OrderStatusSuccess)

	if _, err := repo.FindByIDForUser(ctx, orderID, owner); err != nil {
		t.Fatalf("owner could not read their own order: %v", err)
	}

	_, err := repo.FindByIDForUser(ctx, orderID, stranger)
	if !errors.Is(err, repository.ErrOrderNotFound) {
		t.Fatalf("stranger reading another user's order: error = %v, want ErrOrderNotFound", err)
	}
}

func TestOrderRepositoryCancelRestoresStock(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()

	owner := insertUser(t, db)
	product := insertProduct(t, db, 10)
	orderID := insertOrder(t, db, owner, product, 3, models.OrderStatusSuccess)

	cancelled, err := repo.CancelForUser(ctx, orderID, owner)
	if err != nil {
		t.Fatalf("CancelForUser() error = %v", err)
	}
	if cancelled.Status != models.OrderStatusCancelled {
		t.Errorf("status = %s, want %s", cancelled.Status, models.OrderStatusCancelled)
	}
	if got := productStock(t, db, product); got != 13 {
		t.Errorf("stock = %d, want 13 after restoring 3 units", got)
	}
}

// Cancelling somebody else's order must change nothing at all.
func TestOrderRepositoryCancelIsScopedToOwner(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()

	owner := insertUser(t, db)
	attacker := insertUser(t, db)
	product := insertProduct(t, db, 10)
	orderID := insertOrder(t, db, owner, product, 3, models.OrderStatusSuccess)

	_, err := repo.CancelForUser(ctx, orderID, attacker)
	if !errors.Is(err, repository.ErrOrderNotFound) {
		t.Fatalf("CancelForUser() error = %v, want ErrOrderNotFound", err)
	}
	if got := productStock(t, db, product); got != 10 {
		t.Errorf("stock = %d, want 10: an unauthorised cancellation changed stock", got)
	}

	order, err := repo.FindByIDForUser(ctx, orderID, owner)
	if err != nil {
		t.Fatalf("FindByIDForUser() error = %v", err)
	}
	if order.Status != models.OrderStatusSuccess {
		t.Errorf("status = %s, want %s: an unauthorised cancellation changed the order",
			order.Status, models.OrderStatusSuccess)
	}
}

func TestOrderRepositoryCancelRejectsAlreadyCancelled(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)
	ctx := context.Background()

	owner := insertUser(t, db)
	product := insertProduct(t, db, 10)
	orderID := insertOrder(t, db, owner, product, 3, models.OrderStatusCancelled)

	_, err := repo.CancelForUser(ctx, orderID, owner)
	if !errors.Is(err, repository.ErrOrderNotCancellable) {
		t.Fatalf("CancelForUser() error = %v, want ErrOrderNotCancellable", err)
	}
	if got := productStock(t, db, product); got != 10 {
		t.Errorf("stock = %d, want 10: cancelling an already cancelled order restored stock again", got)
	}
}

// A FAILED order never consumed stock, so cancelling it must not create any.
func TestOrderRepositoryCancelRejectsFailedOrder(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)

	owner := insertUser(t, db)
	product := insertProduct(t, db, 10)
	orderID := insertOrder(t, db, owner, product, 3, models.OrderStatusFailed)

	_, err := repo.CancelForUser(context.Background(), orderID, owner)
	if !errors.Is(err, repository.ErrOrderNotCancellable) {
		t.Fatalf("CancelForUser() error = %v, want ErrOrderNotCancellable", err)
	}
	if got := productStock(t, db, product); got != 10 {
		t.Errorf("stock = %d, want 10: cancelling a failed order invented stock", got)
	}
}

// Concurrent cancellations of one order must restore its stock exactly once.
// Without the conditional UPDATE, each racing request would credit the stock,
// and the sale would oversell.
func TestOrderRepositoryCancelIsExactlyOnceUnderConcurrency(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewOrderRepository(db)

	owner := insertUser(t, db)
	product := insertProduct(t, db, 10)
	orderID := insertOrder(t, db, owner, product, 4, models.OrderStatusSuccess)

	const attempts = 8
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
	)
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines together to maximise contention
			if _, err := repo.CancelForUser(context.Background(), orderID, owner); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if succeeded != 1 {
		t.Errorf("%d of %d concurrent cancellations succeeded, want exactly 1", succeeded, attempts)
	}
	if got := productStock(t, db, product); got != 14 {
		t.Errorf("stock = %d, want 14: stock was restored more than once", got)
	}
}
