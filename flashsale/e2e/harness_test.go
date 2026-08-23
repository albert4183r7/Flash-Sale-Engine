package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

// Ports chosen to avoid clashing with a stack the developer may already be
// running on the default ports.
const (
	gatewayPort  = "18080"
	purchasePort = "18081"
	gatewayURL   = "http://127.0.0.1:" + gatewayPort
	purchaseURL  = "http://127.0.0.1:" + purchasePort

	jwtSecret     = "e2e-jwt-secret-at-least-16-chars"
	internalToken = "e2e-internal-service-token"

	// The seeded demo password; the gateway requires at least 8 characters.
	testPassword = "password123"
)

// stack holds the handles the tests need to inspect the running system.
type stack struct {
	db    *sql.DB
	redis *redis.Client
	http  *http.Client
}

var env *stack

func TestMain(m *testing.M) {
	code, err := runStack(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

// runStack builds and starts the services, runs the tests, then tears
// everything down. It returns 0 with no error when the suite is skipped.
func runStack(m *testing.M) (int, error) {
	postgresURL := os.Getenv("POSTGRES_URL")
	redisAddr := os.Getenv("REDIS_ADDR")
	rabbitURL := os.Getenv("RABBITMQ_URL")

	if postgresURL == "" || redisAddr == "" || rabbitURL == "" {
		fmt.Println("e2e: POSTGRES_URL, REDIS_ADDR and RABBITMQ_URL must all be set; skipping")
		return 0, nil
	}

	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		return 0, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Printf("e2e: PostgreSQL is unreachable (%v); skipping\n", err)
		return 0, nil
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("REDIS_PASSWORD")})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		fmt.Printf("e2e: Redis is unreachable (%v); skipping\n", err)
		return 0, nil
	}

	binDir, err := os.MkdirTemp("", "flashsale-e2e-")
	if err != nil {
		return 0, fmt.Errorf("create build directory: %w", err)
	}
	defer os.RemoveAll(binDir)

	binaries := map[string]string{}
	for _, svc := range []string{"api-gateway", "purchase-service", "order-worker"} {
		path, err := build(svc, binDir)
		if err != nil {
			return 0, err
		}
		binaries[svc] = path
	}

	serviceEnv := append(os.Environ(),
		"JWT_SECRET="+jwtSecret,
		"INTERNAL_API_TOKEN="+internalToken,
		"POSTGRES_URL="+postgresURL,
		"REDIS_ADDR="+redisAddr,
		"RABBITMQ_URL="+rabbitURL,
		"API_GATEWAY_PORT="+gatewayPort,
		"PURCHASE_SERVICE_PORT="+purchasePort,
		"PURCHASE_SERVICE_URL="+purchaseURL,
		// Every request in this suite comes from 127.0.0.1, so a realistic
		// per-client limit would throttle the tests themselves. The limiter is
		// covered precisely by the gateway's middleware tests instead.
		"RATE_LIMIT_REQUESTS=100000",
		"RATE_LIMIT_WINDOW=1m",
		"IDEMPOTENCY_TTL=3s",
	)

	// The worker and the purchase service must be up before the gateway, which
	// health-checks its way to readiness below.
	stopWorker, err := start(binaries["order-worker"], serviceEnv)
	if err != nil {
		return 0, err
	}
	defer stopWorker()

	stopPurchase, err := start(binaries["purchase-service"], serviceEnv)
	if err != nil {
		return 0, err
	}
	defer stopPurchase()

	stopGateway, err := start(binaries["api-gateway"], serviceEnv)
	if err != nil {
		return 0, err
	}
	defer stopGateway()

	client := &http.Client{Timeout: 15 * time.Second}
	if err := waitForHealth(client, gatewayURL+"/health"); err != nil {
		return 0, err
	}
	if err := waitForHealth(client, purchaseURL+"/health"); err != nil {
		return 0, err
	}

	env = &stack{db: db, redis: rdb, http: client}
	return m.Run(), nil
}

// build compiles one service into binDir.
func build(service, binDir string) (string, error) {
	out := filepath.Join(binDir, service)

	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join("..", service)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build %s: %w", service, err)
	}
	return out, nil
}

// start runs a service binary and returns a function that stops it.
func start(binary string, environ []string) (func(), error) {
	cmd := exec.Command(binary)
	cmd.Env = environ
	// Service logs go to the test's stderr, which makes a failure diagnosable.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(binary), err)
	}

	return func() {
		// Signal a graceful shutdown, then wait for the process to exit.
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt)
		}
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
		}
	}, nil
}

// waitForHealth polls an endpoint until it reports healthy.
func waitForHealth(client *http.Client, url string) error {
	deadline := time.Now().Add(60 * time.Second)

	var lastErr error
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("X-Internal-Token", internalToken)

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("%s never became healthy: %w", url, lastErr)
}

// --- request helpers ---

// apiResponse mirrors the shared response envelope.
type apiResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

// call sends a request to the gateway and decodes the envelope.
func call(t *testing.T, method, path, token string, payload any) (int, apiResponse) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, gatewayURL+path, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := env.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	var decoded apiResponse
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s %s returned a non-envelope body (status %d): %s",
				method, path, resp.StatusCode, raw)
		}
	}
	return resp.StatusCode, decoded
}

// --- fixtures ---

// newAccount registers a fresh account and returns its access token and ID.
func newAccount(t *testing.T) (token string, userID uuid.UUID) {
	t.Helper()

	email := "e2e-" + uuid.NewString() + "@example.com"

	status, _ := call(t, http.MethodPost, "/auth/signup", "", map[string]string{
		"email": email, "password": testPassword,
	})
	if status != http.StatusCreated {
		t.Fatalf("signup status = %d, want %d", status, http.StatusCreated)
	}
	t.Cleanup(func() { env.db.Exec(`DELETE FROM users WHERE email = $1`, email) })

	status, body := call(t, http.MethodPost, "/auth/login", "", map[string]string{
		"email": email, "password": testPassword,
	})
	if status != http.StatusOK {
		t.Fatalf("login status = %d, want %d", status, http.StatusOK)
	}

	var login struct {
		Token  string    `json:"token"`
		UserID uuid.UUID `json:"user_id"`
	}
	if err := json.Unmarshal(body.Data, &login); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return login.Token, login.UserID
}

// newProduct creates a product with the given stock, in PostgreSQL and in the
// Redis counter the purchase service reads.
func newProduct(t *testing.T, stock int) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	name := "e2e-product-" + uuid.NewString()
	err := env.db.QueryRow(
		`INSERT INTO products (name, price, stock) VALUES ($1, 100, $2) RETURNING id`,
		name, stock).Scan(&id)
	if err != nil {
		t.Fatalf("insert product: %v", err)
	}

	key := "product_stock:" + id.String()
	if err := env.redis.Set(context.Background(), key, stock, 0).Err(); err != nil {
		t.Fatalf("seed stock counter: %v", err)
	}

	t.Cleanup(func() {
		env.db.Exec(`DELETE FROM orders WHERE product_id = $1`, id)
		env.db.Exec(`DELETE FROM products WHERE id = $1`, id)
		env.redis.Del(context.Background(), key)
	})
	return id
}

// redisStock reads the in-memory counter for a product.
func redisStock(t *testing.T, productID uuid.UUID) int {
	t.Helper()

	value, err := env.redis.Get(context.Background(), "product_stock:"+productID.String()).Int()
	if err != nil {
		t.Fatalf("read stock counter: %v", err)
	}
	return value
}

// dbStock reads the durable stock for a product.
func dbStock(t *testing.T, productID uuid.UUID) int {
	t.Helper()

	var stock int
	if err := env.db.QueryRow(`SELECT stock FROM products WHERE id = $1`, productID).Scan(&stock); err != nil {
		t.Fatalf("read durable stock: %v", err)
	}
	return stock
}

// waitForOrderStatus polls until the order reaches want, which is how the test
// observes the asynchronous worker finishing.
func waitForOrderStatus(t *testing.T, orderID uuid.UUID, want string) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	var last string

	for time.Now().Before(deadline) {
		err := env.db.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&last)
		if err == nil && last == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("order %s status = %q after 20s, want %q", orderID, last, want)
}

// callPurchaseService sends a request straight to the purchase service,
// bypassing the gateway, and returns the status code.
func callPurchaseService(t *testing.T, method, path, token string, payload any) int {
	t.Helper()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, purchaseURL+path, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}

	resp, err := env.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode
}
