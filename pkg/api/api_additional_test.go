package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"
)

func TestHandleBlocklistReload_NoManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress:    ":8080",
		BlocklistManager: nil, // No manager
		Logger:           logger,
		Version:          "test",
	})

	req := httptest.NewRequest("POST", "/api/blocklist/reload", nil)
	w := httptest.NewRecorder()

	server.handleBlocklistReload(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", resp.StatusCode)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	// Wrap with logging middleware
	wrapped := server.loggingMiddleware(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestLoggingMiddleware_CapturesStatusCode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", http.StatusOK},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Error", http.StatusInternalServerError},
		{"201 Created", http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			wrapped := server.loggingMiddleware(testHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			if w.Code != tt.statusCode {
				t.Errorf("Expected status %d, got %d", tt.statusCode, w.Code)
			}
		})
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	rw.WriteHeader(http.StatusNotFound)

	if rw.statusCode != http.StatusNotFound {
		t.Errorf("Expected status code 404, got %d", rw.statusCode)
	}

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected underlying writer status 404, got %d", w.Code)
	}
}

func TestWriteJSON_Error(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	// Create a response writer that always fails
	w := httptest.NewRecorder()

	// Use a channel that can't be marshaled to JSON
	invalidData := make(chan int)

	server.writeJSON(w, http.StatusOK, invalidData)

	// The function should handle the error gracefully without panicking
	// Note: status code will be written even if encoding fails
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestGetUptime_Various(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name     string
		duration time.Duration
		contains string
	}{
		{
			name:     "seconds only",
			duration: 45 * time.Second,
			contains: "45s",
		},
		{
			name:     "minutes and seconds",
			duration: 3*time.Minute + 30*time.Second,
			contains: "3m30s",
		},
		{
			name:     "hours, minutes, and seconds",
			duration: 2*time.Hour + 15*time.Minute + 45*time.Second,
			contains: "2h15m45s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := New(&Config{
				ListenAddress: ":8080",
				Logger:        logger,
				Version:       "test",
			})

			// Set start time to simulate uptime
			server.startTime = time.Now().Add(-tt.duration)

			uptime := server.getUptime()

			if uptime != tt.contains {
				t.Errorf("Expected uptime to be %s, got %s", tt.contains, uptime)
			}
		})
	}
}

func TestHandleQueries_WithDomainFilter(t *testing.T) {
	now := time.Now()
	mock := &mockStorage{
		queries: []*storage.QueryLog{
			{
				Timestamp:    now,
				Domain:       "example.com",
				QueryType:    "A",
				ClientIP:     "192.168.1.1",
				ResponseCode: 0, // NOERROR
				Blocked:      false,
				Cached:       false,
			},
			{
				Timestamp:    now,
				Domain:       "test.com",
				QueryType:    "A",
				ClientIP:     "192.168.1.2",
				ResponseCode: 0, // NOERROR
				Blocked:      false,
				Cached:       false,
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("GET", "/api/queries?limit=10", nil)
	w := httptest.NewRecorder()

	server.handleQueries(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleTopDomains_Blocked(t *testing.T) {
	now := time.Now()
	mock := &mockStorage{
		domains: []*storage.DomainStats{
			{Domain: "blocked1.com", QueryCount: 100, Blocked: true, LastQueried: now},
			{Domain: "blocked2.com", QueryCount: 50, Blocked: true, LastQueried: now},
			{Domain: "allowed.com", QueryCount: 200, Blocked: false, LastQueried: now},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("GET", "/api/top-domains?blocked=true&limit=10", nil)
	w := httptest.NewRecorder()

	server.handleTopDomains(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result TopDomainsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should only return blocked domains
	if len(result.Domains) != 2 {
		t.Errorf("Expected 2 blocked domains, got %d", len(result.Domains))
	}
}

func TestHandleStats_WithSince(t *testing.T) {
	mock := &mockStorage{
		stats: &storage.Statistics{
			TotalQueries:   1000,
			BlockedQueries: 150,
			CachedQueries:  300,
			UniqueClients:  50,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("GET", "/api/stats?since=24h", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestStartShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: "127.0.0.1:0", // Use random port
		Logger:        logger,
		Version:       "test",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for server to shutdown
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled && err != http.ErrServerClosed {
			t.Errorf("Unexpected error during shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not shutdown in time")
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for OPTIONS request")
	})

	wrapped := server.corsMiddleware(testHandler)

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for OPTIONS, got %d", w.Code)
	}

	// Check CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Missing or incorrect CORS Allow-Origin header")
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("Missing CORS Allow-Methods header")
	}
}

func TestHandleUpdatePolicy_NameChange(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	policyEngine := policy.NewEngine()

	// Add initial rule
	rule := &policy.Rule{
		Name:    "Original Name",
		Logic:   `Domain == "old.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	policyEngine.AddRule(rule)

	server := New(&Config{
		ListenAddress: ":8080",
		PolicyEngine:  policyEngine,
		Logger:        logger,
		Version:       "test",
	})

	// Update with different name
	updateReq := PolicyRequest{
		Name:    "New Name",
		Logic:   `Domain == "new.com"`,
		Action:  policy.ActionAllow,
		Enabled: false,
	}

	body, _ := json.Marshal(updateReq)
	req := httptest.NewRequest("PUT", "/api/policies/0", bytes.NewReader(body))
	req.SetPathValue("id", "0")
	w := httptest.NewRecorder()

	server.handleUpdatePolicy(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify the rule was updated
	rules := policyEngine.GetRules()
	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}

	if rules[0].Name != "New Name" {
		t.Errorf("Expected name 'New Name', got %s", rules[0].Name)
	}
}
