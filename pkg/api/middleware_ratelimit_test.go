package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/ratelimit"
)

func TestRateLimitMiddleware_AllowsWhenDisabled(t *testing.T) {
	server := New(&Config{ListenAddress: ":0"})

	nextCalled := false
	handler := server.rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
}

func TestRateLimitMiddleware_LimitsRequests(t *testing.T) {
	logger := logging.NewDefault()
	cfg := &config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             1,
		Action:            config.RateLimitActionDrop,
		CleanupInterval:   0,
		MaxTrackedClients: 100,
	}

	limiter := ratelimit.NewManager(cfg, logger)
	t.Cleanup(func() {
		if limiter != nil {
			limiter.Stop()
		}
	})

	server := New(&Config{ListenAddress: ":0"})
	server.SetHTTPRateLimiter(limiter)

	handler := server.rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "203.0.113.10:4567"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", rr.Code)
	}

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate limited, got %d", rr2.Code)
	}
}

func TestClientIPFromRequestPrefersHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "198.51.100.5:9999"
	req.Header.Set("X-Real-IP", "192.0.2.9")
	req.Header.Set("X-Forwarded-For", "203.0.113.30, 198.51.100.5")

	ip := clientIPFromRequest(req)
	if ip != "203.0.113.30" {
		t.Fatalf("expected forwarded IP, got %s", ip)
	}
}
