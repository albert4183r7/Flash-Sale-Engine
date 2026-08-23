package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/flashsale/common/response"
	"github.com/gin-gonic/gin"
)

// RateLimiter caps how many requests a single client address may make in a
// sliding window. It is in-process, so each gateway instance enforces its own
// limit; that is enough to blunt a single abusive client during a sale.
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

// NewRateLimiter creates a limiter allowing limit requests per window and
// starts the goroutine that evicts idle clients. Call Stop to release it.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
		stop:     make(chan struct{}),
	}
	go rl.evictIdle()
	return rl
}

// Stop ends the eviction goroutine. Without it the limiter's ticker would
// outlive the server it was created for.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stop) })
}

// evictIdle periodically drops clients with no recent requests, so the map does
// not grow without bound as new addresses appear.
func (rl *RateLimiter) evictIdle() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stop:
			return
		case now := <-ticker.C:
			rl.mu.Lock()
			for client, seen := range rl.requests {
				if kept := within(seen, now.Add(-rl.window)); len(kept) == 0 {
					delete(rl.requests, client)
				} else {
					rl.requests[client] = kept
				}
			}
			rl.mu.Unlock()
		}
	}
}

// allow records a request and reports whether it is within the limit.
func (rl *RateLimiter) allow(client string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	recent := within(rl.requests[client], now.Add(-rl.window))
	if len(recent) >= rl.limit {
		rl.requests[client] = recent
		return false
	}

	rl.requests[client] = append(recent, now)
	return true
}

// within returns the timestamps at or after cutoff, reusing the backing array.
func within(times []time.Time, cutoff time.Time) []time.Time {
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	return kept
}

// RateLimit returns the rate limiting middleware.
func (rl *RateLimiter) RateLimit() gin.HandlerFunc {
	retryAfter := strconv.Itoa(int(rl.window.Seconds()))

	return func(c *gin.Context) {
		if !rl.allow(c.ClientIP()) {
			c.Header("Retry-After", retryAfter)
			response.Abort(c, http.StatusTooManyRequests, "Rate limit exceeded",
				"Too many requests. Please try again later.")
			return
		}
		c.Next()
	}
}
