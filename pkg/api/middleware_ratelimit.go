package api

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// rateLimiter implements per-IP token bucket rate limiting.
// It uses a sharded map to reduce contention under high concurrency.
type rateLimiter struct {
	buckets   sync.Map     // map[string]*bucket
	rate      float64      // tokens per second
	burst     int          // max tokens (burst capacity)
	cleanupMu sync.Mutex   // serialize cleanup runs
	lastClean atomic.Int64 // unix seconds of last cleanup
}

type bucket struct {
	tokens   float64
	lastTime int64 // unix nanoseconds
	mu       sync.Mutex
}

func newRateLimiter(requestsPerSecond float64, burst int) *rateLimiter {
	rl := &rateLimiter{
		rate:  requestsPerSecond,
		burst: burst,
	}
	rl.lastClean.Store(time.Now().Unix())
	return rl
}

// allow checks whether a request from the given key should be permitted.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()

	// Lazy cleanup: evict stale entries every 60 seconds
	if now.Unix()-rl.lastClean.Load() > 60 {
		rl.cleanup(now)
	}

	val, _ := rl.buckets.LoadOrStore(key, &bucket{
		tokens:   float64(rl.burst),
		lastTime: now.UnixNano(),
	})
	b := val.(*bucket)

	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := float64(now.UnixNano()-b.lastTime) / float64(time.Second)
	b.lastTime = now.UnixNano()

	// Replenish tokens
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

// cleanup removes entries that haven't been seen in over 5 minutes.
func (rl *rateLimiter) cleanup(now time.Time) {
	if !rl.cleanupMu.TryLock() {
		return // another goroutine is already cleaning
	}
	defer rl.cleanupMu.Unlock()

	threshold := now.Add(-5 * time.Minute).UnixNano()
	rl.buckets.Range(func(key, val any) bool {
		b := val.(*bucket)
		b.mu.Lock()
		stale := b.lastTime < threshold
		b.mu.Unlock()
		if stale {
			rl.buckets.Delete(key)
		}
		return true
	})
	rl.lastClean.Store(now.Unix())
}

// rateLimitMiddleware applies per-IP rate limiting to API requests.
// Login attempts get a strict limit; other API calls get a moderate limit.
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	// Strict limiter for login: 5 req/min (burst 5)
	loginLimiter := newRateLimiter(5.0/60.0, 5)
	// General API limiter: 60 req/sec (burst 120)
	apiLimiter := newRateLimiter(60, 120)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := s.getClientIP(r)

		// Strict rate limit on login
		if r.URL.Path == "/login" && r.Method == http.MethodPost {
			if !loginLimiter.allow(clientIP) {
				w.Header().Set("Retry-After", "60")
				s.writeError(w, http.StatusTooManyRequests, "Too many login attempts")
				return
			}
		}

		// General API rate limit
		if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
			if !apiLimiter.allow(clientIP) {
				w.Header().Set("Retry-After", "1")
				s.writeError(w, http.StatusTooManyRequests, "Rate limit exceeded")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
