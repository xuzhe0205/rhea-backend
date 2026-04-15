package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"rhea-backend/internal/auth"
)

// bucket is a single user's token bucket for one rate-limit group.
// Tokens refill continuously at `rate` tokens/second up to `capacity`.
type bucket struct {
	mu       sync.Mutex
	tokens   float64
	lastTime time.Time
	capacity float64
	rate     float64 // tokens per second
}

// allow attempts to consume one token.
// Returns true if the request is permitted, false if the bucket is empty.
func (b *bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.lastTime = now

	// Refill tokens proportional to elapsed time, capped at capacity.
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// RateLimiter holds per-user token buckets keyed by "userID:group".
// It is safe for concurrent use and is meant to be created once and shared.
type RateLimiter struct {
	buckets sync.Map // key: "<userID>:<group>" → *bucket
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{}
}

// Middleware returns a handler middleware that enforces at most `rpm`
// requests per minute per authenticated user for the given group label.
//
// The group label is an arbitrary string used to namespace the buckets —
// e.g. "chat", "transcribe". Different groups don't share quota.
//
// Must be placed after AuthMiddleware in the chain (it reads the userID
// from context).
func (rl *RateLimiter) Middleware(group string, rpm int) Middleware {
	capacity := float64(rpm)
	rate := capacity / 60.0 // tokens per second

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.GetUserID(r.Context())
			if !ok {
				// Auth middleware should have already rejected this;
				// pass through rather than silently blocking.
				next.ServeHTTP(w, r)
				return
			}

			key := userID.String() + ":" + group

			v, _ := rl.buckets.LoadOrStore(key, &bucket{
				tokens:   capacity, // start full so first requests aren't throttled
				lastTime: time.Now(),
				capacity: capacity,
				rate:     rate,
			})
			b := v.(*bucket)

			if !b.allow() {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "rate limit exceeded — please slow down",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
