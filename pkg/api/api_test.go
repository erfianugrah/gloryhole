package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"glory-hole/pkg/storage"
)

// mockStorage implements storage.Storage for testing
type mockStorage struct {
	stats   *storage.Statistics
	queries []*storage.QueryLog
	domains []*storage.DomainStats
}

func (m *mockStorage) LogQuery(ctx context.Context, query *storage.QueryLog) error {
	return nil
}

func (m *mockStorage) GetRecentQueries(ctx context.Context, limit, offset int) ([]*storage.QueryLog, error) {
	if len(m.queries) > limit {
		return m.queries[:limit], nil
	}
	return m.queries, nil
}

func (m *mockStorage) GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*storage.QueryLog, error) {
	return nil, nil
}

func (m *mockStorage) GetQueriesByClientIP(ctx context.Context, clientIP string, limit int) ([]*storage.QueryLog, error) {
	return nil, nil
}

func (m *mockStorage) GetStatistics(ctx context.Context, since time.Time) (*storage.Statistics, error) {
	return m.stats, nil
}

func (m *mockStorage) GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*storage.DomainStats, error) {
	result := make([]*storage.DomainStats, 0)
	for _, d := range m.domains {
		if d.Blocked == blocked {
			result = append(result, d)
		}
	}
	if len(result) > limit {
		return result[:limit], nil
	}
	return result, nil
}

func (m *mockStorage) GetBlockedCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}

func (m *mockStorage) GetQueryCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}

func (m *mockStorage) GetTraceStatistics(ctx context.Context, since time.Time) (*storage.TraceStatistics, error) {
	return &storage.TraceStatistics{
		Since:    since,
		Until:    time.Now(),
		ByStage:  make(map[string]int64),
		ByAction: make(map[string]int64),
		ByRule:   make(map[string]int64),
		BySource: make(map[string]int64),
	}, nil
}

func (m *mockStorage) GetQueriesWithTraceFilter(ctx context.Context, filter storage.TraceFilter, limit, offset int) ([]*storage.QueryLog, error) {
	return m.queries, nil
}

func (m *mockStorage) Cleanup(ctx context.Context, olderThan time.Time) error {
	return nil
}

func (m *mockStorage) Close() error {
	return nil
}

func (m *mockStorage) Ping(ctx context.Context) error {
	return nil
}

func TestHandleHealth(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
		Version:       "test-version",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf("expected status 'ok', got %s", response.Status)
	}

	if response.Version != "test-version" {
		t.Errorf("expected version 'test-version', got %s", response.Version)
	}

	if response.Uptime == "" {
		t.Error("uptime should not be empty")
	}
}

func TestHandleHealth_MethodNotAllowed(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleStats(t *testing.T) {
	mock := &mockStorage{
		stats: &storage.Statistics{
			TotalQueries:      1000,
			BlockedQueries:    250,
			CachedQueries:     500,
			BlockRate:         25.0,
			CacheHitRate:      50.0,
			AvgResponseTimeMs: 5.0,
		},
	}

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response StatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.TotalQueries != 1000 {
		t.Errorf("expected 1000 total queries, got %d", response.TotalQueries)
	}

	if response.BlockedQueries != 250 {
		t.Errorf("expected 250 blocked queries, got %d", response.BlockedQueries)
	}

	if response.BlockRate != 25.0 {
		t.Errorf("expected 25%% block rate, got %.2f%%", response.BlockRate)
	}

	if response.CacheHitRate != 50.0 {
		t.Errorf("expected 50%% cache hit rate, got %.2f%%", response.CacheHitRate)
	}

	if response.AvgResponseMs != 5.0 {
		t.Errorf("expected 5.0ms avg response time, got %.2f", response.AvgResponseMs)
	}
}

func TestHandleStats_NoStorage(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleQueries(t *testing.T) {
	now := time.Now()
	mock := &mockStorage{
		queries: []*storage.QueryLog{
			{
				ID:             1,
				Timestamp:      now,
				ClientIP:       "192.168.1.100",
				Domain:         "example.com",
				QueryType:      "A",
				ResponseCode:   0,
				Blocked:        false,
				Cached:         true,
				ResponseTimeMs: 5,
				Upstream:       "1.1.1.1:53",
			},
			{
				ID:             2,
				Timestamp:      now.Add(-1 * time.Minute),
				ClientIP:       "192.168.1.101",
				Domain:         "blocked.com",
				QueryType:      "A",
				ResponseCode:   3,
				Blocked:        true,
				Cached:         false,
				ResponseTimeMs: 2,
				Upstream:       "",
			},
		},
	}

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/queries?limit=10", nil)
	w := httptest.NewRecorder()

	server.handleQueries(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response QueriesResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Queries) != 2 {
		t.Errorf("expected 2 queries, got %d", len(response.Queries))
	}

	if response.Queries[0].Domain != "example.com" {
		t.Errorf("expected domain 'example.com', got %s", response.Queries[0].Domain)
	}

	if response.Queries[0].Blocked {
		t.Error("first query should not be blocked")
	}

	if !response.Queries[0].Cached {
		t.Error("first query should be cached")
	}

	if !response.Queries[1].Blocked {
		t.Error("second query should be blocked")
	}
}

func TestHandleTopDomains(t *testing.T) {
	mock := &mockStorage{
		domains: []*storage.DomainStats{
			{Domain: "example.com", QueryCount: 100, Blocked: false},
			{Domain: "test.com", QueryCount: 50, Blocked: false},
			{Domain: "blocked.com", QueryCount: 75, Blocked: true},
			{Domain: "malware.com", QueryCount: 25, Blocked: true},
		},
	}

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
	})

	// Test non-blocked domains
	req := httptest.NewRequest(http.MethodGet, "/api/top-domains?limit=5&blocked=false", nil)
	w := httptest.NewRecorder()

	server.handleTopDomains(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response TopDomainsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Domains) != 2 {
		t.Errorf("expected 2 non-blocked domains, got %d", len(response.Domains))
	}

	// Test blocked domains
	req = httptest.NewRequest(http.MethodGet, "/api/top-domains?limit=5&blocked=true", nil)
	w = httptest.NewRecorder()

	server.handleTopDomains(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Domains) != 2 {
		t.Errorf("expected 2 blocked domains, got %d", len(response.Domains))
	}

	if !response.Domains[0].Blocked {
		t.Error("expected blocked domains only")
	}
}

func TestCORSMiddleware(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	w := httptest.NewRecorder()

	server.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for OPTIONS, got %d", w.Code)
	}

	corsHeader := w.Header().Get("Access-Control-Allow-Origin")
	if corsHeader != "*" {
		t.Errorf("expected CORS header '*', got %s", corsHeader)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		fallback time.Duration
		expected time.Duration
	}{
		{"1h", 24 * time.Hour, 1 * time.Hour},
		{"30m", 24 * time.Hour, 30 * time.Minute},
		{"", 24 * time.Hour, 24 * time.Hour},
		{"invalid", 24 * time.Hour, 24 * time.Hour},
	}

	for _, tt := range tests {
		result := parseDuration(tt.input, tt.fallback)
		if result != tt.expected {
			t.Errorf("parseDuration(%q, %v) = %v, expected %v",
				tt.input, tt.fallback, result, tt.expected)
		}
	}
}

func TestGetUptime(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	// Wait a bit to have non-zero uptime
	time.Sleep(100 * time.Millisecond)

	uptime := server.getUptime()
	if uptime == "" {
		t.Error("uptime should not be empty")
	}

	// Should be in format like "0s" or "1s"
	if len(uptime) < 2 {
		t.Errorf("uptime format unexpected: %s", uptime)
	}
}
