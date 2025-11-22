package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"glory-hole/pkg/storage"
)

func TestHandleDashboard(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	server.handleDashboard(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Expected text/html content type, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestHandleDashboard_WrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()

	server.handleDashboard(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandleQueriesPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
	})

	req := httptest.NewRequest("GET", "/queries", nil)
	w := httptest.NewRecorder()

	server.handleQueriesPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleQueriesPage_WrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("DELETE", "/queries", nil)
	w := httptest.NewRecorder()

	server.handleQueriesPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandlePoliciesPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
	})

	req := httptest.NewRequest("GET", "/policies", nil)
	w := httptest.NewRecorder()

	server.handlePoliciesPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleSettingsPage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
	})

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()

	server.handleSettingsPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleStatsPartial_NoStorage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       nil, // No storage
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("GET", "/api/ui/stats", nil)
	w := httptest.NewRecorder()

	server.handleStatsPartial(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleStatsPartial_WithStorage(t *testing.T) {
	now := time.Now()
	mock := &mockStorage{
		stats: &storage.Statistics{
			TotalQueries:      1000,
			BlockedQueries:    250,
			CachedQueries:     500,
			BlockRate:         25.0,
			CacheHitRate:      50.0,
			AvgResponseTimeMs: 15.5,
			Since:             now.Add(-24 * time.Hour),
			Until:             now,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("GET", "/api/ui/stats?since=24h", nil)
	w := httptest.NewRecorder()

	server.handleStatsPartial(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleStatsPartial_WrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("POST", "/api/ui/stats", nil)
	w := httptest.NewRecorder()

	server.handleStatsPartial(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandleTopDomainsPartial_NoStorage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       nil,
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("GET", "/api/ui/top-domains?limit=10&blocked=false", nil)
	w := httptest.NewRecorder()

	server.handleTopDomainsPartial(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleTopDomainsPartial_WithStorage(t *testing.T) {
	now := time.Now()
	mock := &mockStorage{
		domains: []*storage.DomainStats{
			{Domain: "example.com", QueryCount: 100, Blocked: false, LastQueried: now},
			{Domain: "test.com", QueryCount: 75, Blocked: false, LastQueried: now},
			{Domain: "blocked.com", QueryCount: 50, Blocked: true, LastQueried: now},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
		Logger:        logger,
		Version:       "test",
	})

	tests := []struct {
		name    string
		url     string
		blocked bool
	}{
		{"allowed domains", "/api/ui/top-domains?limit=10&blocked=false", false},
		{"blocked domains", "/api/ui/top-domains?limit=5&blocked=true", true},
		{"default params", "/api/ui/top-domains", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()

			server.handleTopDomainsPartial(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestHandleTopDomainsPartial_WrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("PUT", "/api/ui/top-domains", nil)
	w := httptest.NewRecorder()

	server.handleTopDomainsPartial(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandleQueriesPartial_NoStorage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       nil,
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("GET", "/api/ui/queries?limit=20", nil)
	w := httptest.NewRecorder()

	server.handleQueriesPartial(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleQueriesPartial_WithStorage(t *testing.T) {
	now := time.Now()
	mock := &mockStorage{
		queries: []*storage.QueryLog{
			{
				Timestamp:      now,
				ClientIP:       "192.168.1.100",
				Domain:         "example.com",
				QueryType:      "A",
				Blocked:        false,
				Cached:         true,
				ResponseTimeMs: 10,
			},
			{
				Timestamp:      now.Add(-1 * time.Minute),
				ClientIP:       "192.168.1.101",
				Domain:         "blocked.com",
				QueryType:      "A",
				Blocked:        true,
				Cached:         false,
				ResponseTimeMs: 5,
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

	tests := []struct {
		name  string
		url   string
		limit int
	}{
		{"default limit", "/api/ui/queries", 20},
		{"custom limit", "/api/ui/queries?limit=50", 50},
		{"invalid limit", "/api/ui/queries?limit=invalid", 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()

			server.handleQueriesPartial(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestHandleQueriesPartial_WrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test",
	})

	req := httptest.NewRequest("DELETE", "/api/ui/queries", nil)
	w := httptest.NewRecorder()

	server.handleQueriesPartial(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestInitTemplates(t *testing.T) {
	err := initTemplates()
	if err != nil {
		t.Fatalf("initTemplates() failed: %v", err)
	}

	if templates == nil {
		t.Error("templates should not be nil after initialization")
	}

	// Test that all expected templates exist
	expectedTemplates := []string{
		"base.html",
		"dashboard.html",
		"queries.html",
		"policies.html",
		"settings.html",
		"stats_partial.html",
		"queries_partial.html",
		"top_domains_partial.html",
		"policies_partial.html",
	}

	for _, name := range expectedTemplates {
		tmpl := templates.Lookup(name)
		if tmpl == nil {
			t.Errorf("Expected template %s not found", name)
		}
	}
}

func TestGetStaticFS(t *testing.T) {
	fs, err := getStaticFS()
	if err != nil {
		t.Fatalf("getStaticFS() failed: %v", err)
	}

	if fs == nil {
		t.Error("staticFS should not be nil")
	}

	// Try to open a known file
	file, err := fs.Open("css/style.css")
	if err != nil {
		t.Errorf("Failed to open static file css/style.css: %v", err)
	} else {
		defer func() { _ = file.Close() }()
	}
}

// Mock storage for testing UI handlers with data
type mockUIStorage struct {
	stats   *storage.Statistics
	domains []*storage.DomainStats
	queries []*storage.QueryLog
}

func (m *mockUIStorage) LogQuery(ctx context.Context, query *storage.QueryLog) error {
	return nil
}

func (m *mockUIStorage) GetRecentQueries(ctx context.Context, limit int) ([]*storage.QueryLog, error) {
	if m.queries == nil {
		return []*storage.QueryLog{}, nil
	}
	return m.queries, nil
}

func (m *mockUIStorage) GetStatistics(ctx context.Context, since time.Time) (*storage.Statistics, error) {
	if m.stats == nil {
		return &storage.Statistics{}, nil
	}
	return m.stats, nil
}

func (m *mockUIStorage) GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*storage.DomainStats, error) {
	if m.domains == nil {
		return []*storage.DomainStats{}, nil
	}

	filtered := []*storage.DomainStats{}
	for _, d := range m.domains {
		if d.Blocked == blocked {
			filtered = append(filtered, d)
		}
	}

	if len(filtered) > limit {
		return filtered[:limit], nil
	}
	return filtered, nil
}

func (m *mockUIStorage) Close() error {
	return nil
}
