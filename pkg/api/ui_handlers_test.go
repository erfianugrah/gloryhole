package api

import (
	"context"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"glory-hole/pkg/config"
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

	defaultCfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress:   ":1053",
			WebUIAddress:    ":8081",
			TCPEnabled:      true,
			UDPEnabled:      true,
			EnableBlocklist: true,
			EnablePolicies:  true,
		},
		Cache: config.CacheConfig{
			Enabled:     true,
			MaxEntries:  2000,
			MinTTL:      time.Minute,
			MaxTTL:      12 * time.Hour,
			NegativeTTL: 10 * time.Minute,
			BlockedTTL:  2 * time.Second,
			ShardCount:  8,
		},
		Policy: config.PolicyConfig{
			Enabled: true,
			Rules: []config.PolicyRuleEntry{
				{Name: "Test", Logic: "true", Action: "BLOCK", Enabled: true},
			},
		},
		RateLimit: config.RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 150,
			Burst:             300,
			Action:            config.RateLimitActionNXDOMAIN,
			LogViolations:     true,
			CleanupInterval:   2 * time.Minute,
			MaxTrackedClients: 500,
		},
		Telemetry: config.TelemetryConfig{
			ServiceName:       "glory-hole",
			ServiceVersion:    "test",
			PrometheusEnabled: true,
			PrometheusPort:    9100,
			Enabled:           true,
		},
		Database: storage.Config{
			Backend:       storage.BackendSQLite,
			BufferSize:    500,
			RetentionDays: 5,
		},
		UpstreamDNSServers:   []string{"9.9.9.9:53"},
		Blocklists:           []string{"https://example.com/block.txt"},
		Whitelist:            []string{"allowed.test"},
		AutoUpdateBlocklists: true,
		UpdateInterval:       6 * time.Hour,
	}
	server := New(&Config{
		ListenAddress: ":8080",
		Logger:        logger,
		Version:       "test-1.0.0",
		ConfigPath:    "/etc/glory-hole/config.yml",
		InitialConfig: defaultCfg,
	})

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()

	server.handleSettingsPage(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), defaultCfg.Server.ListenAddress) {
		t.Errorf("expected settings page to include listen address %s", defaultCfg.Server.ListenAddress)
	}
	if !strings.Contains(string(body), "/etc/glory-hole/config.yml") {
		t.Errorf("expected settings page to include config path")
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

	pageTemplates := map[string]*template.Template{
		"dashboard.html": dashboardTemplate,
		"queries.html":   queriesTemplate,
		"policies.html":  policiesTemplate,
		"settings.html":  settingsTemplate,
	}

	for name, tmpl := range pageTemplates {
		if tmpl == nil {
			t.Fatalf("Expected %s template to be initialized", name)
		}
		if tmpl.Lookup(name) == nil {
			t.Errorf("Template %s missing entrypoint %s", name, name)
		}
	}

	partialTemplates := map[string]*template.Template{
		"stats_partial.html":   statsPartialTemplate,
		"queries_partial.html": queriesPartialTemplate,
		"top_domains_partial.html": topDomainsTemplate,
		"policies_partial.html":    policiesPartialTemplate,
	}

	for name, tmpl := range partialTemplates {
		if tmpl == nil {
			t.Fatalf("Expected %s template to be initialized", name)
		}
		if tmpl.Lookup(name) == nil {
			t.Errorf("Standalone template %s missing definition", name)
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

	// Try to open known files
	files := []string{"css/style.css", "js/mini-htmx.js"}
	for _, path := range files {
		f, err := fs.Open(path)
		if err != nil {
			t.Errorf("Failed to open static file %s: %v", path, err)
			continue
		}
		_ = f.Close()
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

func (m *mockUIStorage) GetRecentQueries(ctx context.Context, limit, offset int) ([]*storage.QueryLog, error) {
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

func (m *mockUIStorage) GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*storage.QueryLog, error) {
	return nil, nil
}

func (m *mockUIStorage) GetQueriesByClientIP(ctx context.Context, clientIP string, limit int) ([]*storage.QueryLog, error) {
	return nil, nil
}

func (m *mockUIStorage) GetBlockedCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}

func (m *mockUIStorage) GetQueryCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}

func (m *mockUIStorage) GetTimeSeriesStats(ctx context.Context, bucket time.Duration, points int) ([]*storage.TimeSeriesPoint, error) {
	return []*storage.TimeSeriesPoint{}, nil
}

func (m *mockUIStorage) GetQueriesFiltered(ctx context.Context, filter storage.QueryFilter, limit, offset int) ([]*storage.QueryLog, error) {
	return m.queries, nil
}

func (m *mockUIStorage) GetTraceStatistics(ctx context.Context, since time.Time) (*storage.TraceStatistics, error) {
	return &storage.TraceStatistics{
		Since:    since,
		Until:    time.Now(),
		ByStage:  make(map[string]int64),
		ByAction: make(map[string]int64),
		ByRule:   make(map[string]int64),
		BySource: make(map[string]int64),
	}, nil
}

func (m *mockUIStorage) GetQueriesWithTraceFilter(ctx context.Context, filter storage.TraceFilter, limit, offset int) ([]*storage.QueryLog, error) {
	return m.queries, nil
}

func (m *mockUIStorage) Cleanup(ctx context.Context, olderThan time.Time) error {
	return nil
}

func (m *mockUIStorage) Ping(ctx context.Context) error {
	return nil
}

func (m *mockUIStorage) Close() error {
	return nil
}
