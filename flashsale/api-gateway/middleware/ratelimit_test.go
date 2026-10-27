package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/flashsale/api-gateway/middleware"
	"github.com/gin-gonic/gin"
)

// newLimitedRouter returns a router that allows limit requests per window.
func newLimitedRouter(t *testing.T, limit int, window time.Duration) *gin.Engine {
	t.Helper()

	rl := middleware.NewRateLimiter(limit, window)
	t.Cleanup(rl.Stop)

	r := gin.New()
	r.Use(rl.RateLimit())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func do(r *gin.Engine, clientIP string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = clientIP + ":12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRateLimitAllowsUpToLimit(t *testing.T) {
	const limit = 5
	r := newLimitedRouter(t, limit, time.Minute)

	for i := 1; i <= limit; i++ {
		if got := do(r, "10.0.0.1").Code; got != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i, got, http.StatusOK)
		}
	}

	w := do(r, "10.0.0.1")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request %d: status = %d, want %d", limit+1, w.Code, http.StatusTooManyRequests)
	}
	// Clients need to know when to come back.
	if retry := w.Header().Get("Retry-After"); retry == "" {
		t.Error("Retry-After header is missing from a rate limited response")
	} else if _, err := strconv.Atoi(retry); err != nil {
		t.Errorf("Retry-After = %q, want a number of seconds", retry)
	}
}

// One noisy client must not be able to lock everyone else out.
func TestRateLimitIsPerClient(t *testing.T) {
	const limit = 2
	r := newLimitedRouter(t, limit, time.Minute)

	for i := 0; i < limit+1; i++ {
		do(r, "10.0.0.1")
	}

	if got := do(r, "10.0.0.2").Code; got != http.StatusOK {
		t.Fatalf("second client: status = %d, want %d", got, http.StatusOK)
	}
}

// The window slides: once old requests age out, the client is served again.
func TestRateLimitWindowExpires(t *testing.T) {
	const window = 100 * time.Millisecond
	r := newLimitedRouter(t, 1, window)

	if got := do(r, "10.0.0.3").Code; got != http.StatusOK {
		t.Fatalf("first request: status = %d, want %d", got, http.StatusOK)
	}
	if got := do(r, "10.0.0.3").Code; got != http.StatusTooManyRequests {
		t.Fatalf("second request: status = %d, want %d", got, http.StatusTooManyRequests)
	}

	time.Sleep(window + 50*time.Millisecond)

	if got := do(r, "10.0.0.3").Code; got != http.StatusOK {
		t.Fatalf("after the window: status = %d, want %d", got, http.StatusOK)
	}
}

// The limiter is shared by every request the server handles, so its state must
// be safe under concurrency. Run with -race to catch unsynchronised access.
func TestRateLimitConcurrentAccess(t *testing.T) {
	const (
		limit   = 50
		clients = 8
		each    = 25
	)
	r := newLimitedRouter(t, limit, time.Minute)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	for c := 0; c < clients; c++ {
		for i := 0; i < each; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Every goroutine uses the same client address, so they all
				// contend for one bucket.
				if do(r, "10.0.0.9").Code == http.StatusOK {
					mu.Lock()
					allowed++
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()

	// The limit must hold exactly: a lost update would let extra requests
	// through.
	if allowed != limit {
		t.Fatalf("allowed %d requests, want exactly %d", allowed, limit)
	}
}

// Stop must be safe to call more than once, since shutdown paths can overlap.
func TestRateLimiterStopIsIdempotent(t *testing.T) {
	rl := middleware.NewRateLimiter(1, time.Minute)
	rl.Stop()
	rl.Stop()
}
