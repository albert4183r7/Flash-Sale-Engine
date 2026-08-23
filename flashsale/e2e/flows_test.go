package e2e

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// decodeOrderID pulls the order ID out of a purchase response.
func decodeOrderID(t *testing.T, data json.RawMessage) uuid.UUID {
	t.Helper()

	var body struct {
		OrderID uuid.UUID `json:"order_id"`
		Status  string    `json:"status"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("decode purchase response: %v", err)
	}
	if body.OrderID == uuid.Nil {
		t.Fatal("purchase response carried no order id")
	}
	return body.OrderID
}

// buy places an order and returns the status and order ID.
func buy(t *testing.T, token string, productID uuid.UUID, qty int) (int, uuid.UUID) {
	t.Helper()

	status, body := call(t, http.MethodPost, "/purchase", token, map[string]any{
		"product_id": productID, "qty": qty,
	})
	if status != http.StatusAccepted {
		return status, uuid.Nil
	}
	return status, decodeOrderID(t, body.Data)
}

// The core business flow: a buyer signs up, logs in, orders, and the order is
// persisted asynchronously with stock deducted in both Redis and PostgreSQL.
func TestPurchaseFlowPersistsOrderAndStock(t *testing.T) {
	token, userID := newAccount(t)
	product := newProduct(t, 10)

	status, orderID := buy(t, token, product, 3)
	if status != http.StatusAccepted {
		t.Fatalf("purchase status = %d, want %d", status, http.StatusAccepted)
	}

	// Stock is reserved in Redis before the response is returned.
	if got := redisStock(t, product); got != 7 {
		t.Errorf("stock counter = %d, want 7", got)
	}

	// The worker then persists the order and deducts durable stock.
	waitForOrderStatus(t, orderID, "SUCCESS")
	if got := dbStock(t, product); got != 7 {
		t.Errorf("durable stock = %d, want 7", got)
	}

	// The buyer can read their own order back.
	statusCode, body := call(t, http.MethodGet, "/orders/"+orderID.String(), token, nil)
	if statusCode != http.StatusOK {
		t.Fatalf("get order status = %d, want %d", statusCode, http.StatusOK)
	}

	var order struct {
		OrderID uuid.UUID `json:"order_id"`
		UserID  uuid.UUID `json:"user_id"`
		Qty     int       `json:"qty"`
		Status  string    `json:"status"`
	}
	if err := json.Unmarshal(body.Data, &order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if order.OrderID != orderID || order.UserID != userID || order.Qty != 3 || order.Status != "SUCCESS" {
		t.Errorf("order = %+v, want order %s for user %s, qty 3, SUCCESS", order, orderID, userID)
	}
}

// Cancelling returns the stock to both stores and marks the order cancelled.
func TestCancelFlowRestoresStock(t *testing.T) {
	token, _ := newAccount(t)
	product := newProduct(t, 10)

	_, orderID := buy(t, token, product, 4)
	waitForOrderStatus(t, orderID, "SUCCESS")

	status, _ := call(t, http.MethodDelete, "/orders/"+orderID.String(), token, nil)
	if status != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d", status, http.StatusOK)
	}

	if got := dbStock(t, product); got != 10 {
		t.Errorf("durable stock = %d, want 10", got)
	}
	if got := redisStock(t, product); got != 10 {
		t.Errorf("stock counter = %d, want 10", got)
	}

	// Cancelling again must be refused rather than returning the stock twice.
	status, _ = call(t, http.MethodDelete, "/orders/"+orderID.String(), token, nil)
	if status != http.StatusConflict {
		t.Fatalf("second cancel status = %d, want %d", status, http.StatusConflict)
	}
	if got := dbStock(t, product); got != 10 {
		t.Errorf("durable stock = %d, want 10 after a refused second cancellation", got)
	}
}

// One buyer must not be able to read or cancel another buyer's order. This is
// the end-to-end regression test for the access-control flaw that let any
// authenticated user act on any order by ID.
func TestOrdersArePrivateToTheirOwner(t *testing.T) {
	ownerToken, _ := newAccount(t)
	attackerToken, _ := newAccount(t)
	product := newProduct(t, 10)

	_, orderID := buy(t, ownerToken, product, 2)
	waitForOrderStatus(t, orderID, "SUCCESS")

	t.Run("read", func(t *testing.T) {
		status, _ := call(t, http.MethodGet, "/orders/"+orderID.String(), attackerToken, nil)
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want %d: another user could read this order", status, http.StatusNotFound)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		status, _ := call(t, http.MethodDelete, "/orders/"+orderID.String(), attackerToken, nil)
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want %d: another user could cancel this order", status, http.StatusNotFound)
		}
		if got := dbStock(t, product); got != 8 {
			t.Errorf("durable stock = %d, want 8: an unauthorised cancellation changed stock", got)
		}

		var stored string
		env.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&stored)
		if stored != "SUCCESS" {
			t.Errorf("order status = %q, want %q: an unauthorised cancellation changed it", stored, "SUCCESS")
		}
	})
}

// Resubmitting the same order within the idempotency window is refused, and
// does not consume stock a second time.
func TestDuplicatePurchaseIsRefused(t *testing.T) {
	token, _ := newAccount(t)
	product := newProduct(t, 10)

	if status, _ := buy(t, token, product, 1); status != http.StatusAccepted {
		t.Fatalf("first purchase status = %d, want %d", status, http.StatusAccepted)
	}

	status, _ := call(t, http.MethodPost, "/purchase", token, map[string]any{
		"product_id": product, "qty": 1,
	})
	if status != http.StatusConflict {
		t.Fatalf("duplicate purchase status = %d, want %d", status, http.StatusConflict)
	}
	if got := redisStock(t, product); got != 9 {
		t.Errorf("stock counter = %d, want 9: the duplicate consumed stock", got)
	}
}

func TestPurchaseRejectsInvalidRequests(t *testing.T) {
	token, _ := newAccount(t)
	product := newProduct(t, 10)

	tests := []struct {
		name    string
		payload map[string]any
		want    int
	}{
		{"zero qty", map[string]any{"product_id": product, "qty": 0}, http.StatusBadRequest},
		{"negative qty", map[string]any{"product_id": product, "qty": -1}, http.StatusBadRequest},
		{"over the per-order cap", map[string]any{"product_id": product, "qty": 11}, http.StatusBadRequest},
		{"missing product", map[string]any{"qty": 1}, http.StatusBadRequest},
		{"malformed product id", map[string]any{"product_id": "not-a-uuid", "qty": 1}, http.StatusBadRequest},
		{"nil product id", map[string]any{"product_id": uuid.Nil, "qty": 1}, http.StatusBadRequest},
		{"unknown product", map[string]any{"product_id": uuid.New(), "qty": 1}, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := call(t, http.MethodPost, "/purchase", token, tt.payload)
			if status != tt.want {
				t.Fatalf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

// Ordering more than remains must be refused outright, leaving stock untouched.
func TestPurchaseRefusesWhenStockIsShort(t *testing.T) {
	token, _ := newAccount(t)
	product := newProduct(t, 2)

	status, _ := call(t, http.MethodPost, "/purchase", token, map[string]any{
		"product_id": product, "qty": 5,
	})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want %d", status, http.StatusConflict)
	}
	if got := redisStock(t, product); got != 2 {
		t.Errorf("stock counter = %d, want 2", got)
	}
}

// Every protected route must reject an unauthenticated or forged caller.
func TestProtectedRoutesRequireAuthentication(t *testing.T) {
	orderPath := "/orders/" + uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodPost, "/purchase"},
		{http.MethodGet, orderPath},
		{http.MethodDelete, orderPath},
	}
	tokens := []struct{ name, token string }{
		{"no token", ""},
		{"garbage token", "not.a.jwt"},
		// Correctly formed but signed with the wrong key.
		{"foreign token", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
			"eyJ1c2VyX2lkIjoiMDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAxIiwiZXhwIjo0MTAyNDQ0ODAwfQ." +
			"c2lnbmF0dXJlLXRoYXQtd2lsbC1uZXZlci12ZXJpZnk"},
	}

	for _, route := range routes {
		for _, tk := range tokens {
			t.Run(route.method+" "+route.path+" "+tk.name, func(t *testing.T) {
				status, _ := call(t, route.method, route.path, tk.token, map[string]any{
					"product_id": uuid.New(), "qty": 1,
				})
				if status != http.StatusUnauthorized {
					t.Fatalf("status = %d, want %d", status, http.StatusUnauthorized)
				}
			})
		}
	}
}

// The purchase service trusts the user ID in its request bodies, so it must be
// unreachable without the internal token. Without this guard anyone able to
// reach the service could order as any user or inflate stock at will.
func TestPurchaseServiceRejectsUntrustedCallers(t *testing.T) {
	product := newProduct(t, 10)

	requests := []struct {
		name, method, path string
		payload            any
	}{
		{"purchase", http.MethodPost, "/purchase", map[string]any{
			"user_id": uuid.New(), "product_id": product, "qty": 1,
		}},
		{"restore stock", http.MethodPost, "/internal/stock/restore", map[string]any{
			"product_id": product, "qty": 999999,
		}},
		{"read stock", http.MethodGet, "/stock/" + product.String(), nil},
	}

	for _, tt := range requests {
		t.Run(tt.name, func(t *testing.T) {
			status := callPurchaseService(t, tt.method, tt.path, "", tt.payload)
			if status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", status, http.StatusUnauthorized)
			}
		})
	}

	// The stock must be untouched by the rejected attempts.
	if got := redisStock(t, product); got != 10 {
		t.Errorf("stock counter = %d, want 10: an unauthenticated caller changed stock", got)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	token, _ := newAccount(t)
	if token == "" {
		t.Fatal("expected an access token")
	}

	tests := []struct {
		name    string
		payload map[string]string
		want    int
	}{
		{"wrong password", map[string]string{"email": "e2e-nobody@example.com", "password": "wrong-password"}, http.StatusUnauthorized},
		{"unknown account", map[string]string{"email": "e2e-absent@example.com", "password": testPassword}, http.StatusUnauthorized},
		{"malformed email", map[string]string{"email": "not-an-email", "password": testPassword}, http.StatusBadRequest},
		{"missing password", map[string]string{"email": "e2e-nobody@example.com"}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := call(t, http.MethodPost, "/auth/login", "", tt.payload)
			if status != tt.want {
				t.Fatalf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

func TestSignupRejectsDuplicateAndWeakPasswords(t *testing.T) {
	email := "e2e-" + uuid.NewString() + "@example.com"
	t.Cleanup(func() { env.db.Exec(`DELETE FROM users WHERE email = $1`, email) })

	status, _ := call(t, http.MethodPost, "/auth/signup", "", map[string]string{
		"email": email, "password": testPassword,
	})
	if status != http.StatusCreated {
		t.Fatalf("signup status = %d, want %d", status, http.StatusCreated)
	}

	status, _ = call(t, http.MethodPost, "/auth/signup", "", map[string]string{
		"email": email, "password": testPassword,
	})
	if status != http.StatusConflict {
		t.Fatalf("duplicate signup status = %d, want %d", status, http.StatusConflict)
	}

	status, _ = call(t, http.MethodPost, "/auth/signup", "", map[string]string{
		"email": "e2e-" + uuid.NewString() + "@example.com", "password": "short",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("weak password signup status = %d, want %d", status, http.StatusBadRequest)
	}
}

// The system's central promise: however many buyers arrive at once, it sells
// exactly the available stock and never a unit more.
func TestConcurrentBuyersNeverOversell(t *testing.T) {
	const (
		available = 15
		buyers    = 60
	)
	product := newProduct(t, available)

	// Each buyer needs their own account, because the idempotency claim is per
	// buyer and product.
	tokens := make([]string, buyers)
	for i := range tokens {
		tokens[i], _ = newAccount(t)
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted []uuid.UUID
	)
	start := make(chan struct{})

	for _, token := range tokens {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			<-start
			status, body := call(t, http.MethodPost, "/purchase", token, map[string]any{
				"product_id": product, "qty": 1,
			})
			if status == http.StatusAccepted {
				mu.Lock()
				accepted = append(accepted, decodeOrderID(t, body.Data))
				mu.Unlock()
			}
		}(token)
	}
	close(start)
	wg.Wait()

	if len(accepted) != available {
		t.Fatalf("%d purchases were accepted, want exactly %d", len(accepted), available)
	}
	if got := redisStock(t, product); got != 0 {
		t.Errorf("stock counter = %d, want 0", got)
	}

	for _, orderID := range accepted {
		waitForOrderStatus(t, orderID, "SUCCESS")
	}

	// Every accepted order must be durably recorded, and durable stock must
	// agree with what was sold.
	var persisted int
	if err := env.db.QueryRow(
		`SELECT count(*) FROM orders WHERE product_id = $1 AND status = 'SUCCESS'`,
		product).Scan(&persisted); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if persisted != available {
		t.Errorf("%d orders were persisted, want %d", persisted, available)
	}
	if got := dbStock(t, product); got != 0 {
		t.Errorf("durable stock = %d, want 0", got)
	}
}

// The idempotency claim expires, so a buyer may order the same product again
// after the window passes.
func TestPurchaseAllowedAgainAfterIdempotencyWindow(t *testing.T) {
	token, _ := newAccount(t)
	product := newProduct(t, 10)

	if status, _ := buy(t, token, product, 1); status != http.StatusAccepted {
		t.Fatal("first purchase was not accepted")
	}

	// IDEMPOTENCY_TTL is set to 3s for the test stack.
	time.Sleep(4 * time.Second)

	if status, _ := buy(t, token, product, 1); status != http.StatusAccepted {
		t.Fatalf("purchase after the idempotency window was refused")
	}
	if got := redisStock(t, product); got != 8 {
		t.Errorf("stock counter = %d, want 8", got)
	}
}

func TestHealthEndpoint(t *testing.T) {
	status, body := call(t, http.MethodGet, "/health", "", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if !body.Success {
		t.Errorf("health reported success = false: %+v", body)
	}
}
