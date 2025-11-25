package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/storage"
)

// mockStorage implements storage.Storage for testing
type mockStorage struct {
	stats      *storage.Statistics
	queries    []*storage.QueryLog
	domains    []*storage.DomainStats
	timeseries []*storage.TimeSeriesPoint
	queryTypes []*storage.QueryTypeStats
	filtered   []*storage.QueryLog
	lastFilter storage.QueryFilter
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

func (m *mockStorage) GetQueriesFiltered(ctx context.Context, filter storage.QueryFilter, limit, offset int) ([]*storage.QueryLog, error) {
	m.lastFilter = filter
	if len(m.filtered) > 0 {
		return m.filtered, nil
	}
	if len(m.queries) > 0 {
		return m.queries, nil
	}
	return []*storage.QueryLog{}, nil
}

func (m *mockStorage) GetTimeSeriesStats(ctx context.Context, bucket time.Duration, points int) ([]*storage.TimeSeriesPoint, error) {
	if len(m.timeseries) == 0 {
		return []*storage.TimeSeriesPoint{}, nil
	}
	return m.timeseries, nil
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

func (m *mockStorage) GetQueryTypeStats(ctx context.Context, limit int) ([]*storage.QueryTypeStats, error) {
	if m.queryTypes != nil {
		return m.queryTypes, nil
	}
	return []*storage.QueryTypeStats{}, nil
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

func newConfigTestServer(t *testing.T, mutate func(*config.Config)) (*Server, string) {
	t.Helper()

	cfg := config.LoadWithDefaults()
	if mutate != nil {
		mutate(cfg)
	}

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	server := New(&Config{
		ListenAddress: ":0",
		InitialConfig: cfg,
		ConfigPath:    path,
	})

	return server, path
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

func TestHandleQueryTypes(t *testing.T) {
	mock := &mockStorage{
		queryTypes: []*storage.QueryTypeStats{
			{QueryType: "A", Total: 10, Blocked: 2, Cached: 5},
			{QueryType: "AAAA", Total: 5, Blocked: 1, Cached: 1},
		},
	}

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/stats/query-types?limit=5", nil)
	w := httptest.NewRecorder()

	server.handleQueryTypes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp QueryTypeStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Types) != 2 || resp.Types[0].QueryType != "A" || resp.Types[0].Total != 10 {
		t.Fatalf("unexpected response payload: %+v", resp)
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

func TestHandleGetConfig(t *testing.T) {
	initial := &config.Config{
		Server: config.ServerConfig{
			ListenAddress:   ":53",
			WebUIAddress:    ":8080",
			TCPEnabled:      true,
			UDPEnabled:      true,
			EnableBlocklist: true,
			EnablePolicies:  false,
		},
		Cache: config.CacheConfig{
			Enabled:     true,
			MaxEntries:  1000,
			MinTTL:      time.Minute,
			MaxTTL:      24 * time.Hour,
			NegativeTTL: 5 * time.Minute,
			BlockedTTL:  time.Second,
			ShardCount:  16,
		},
		Policy: config.PolicyConfig{
			Enabled: true,
			Rules: []config.PolicyRuleEntry{
				{Name: "Test", Logic: "true", Action: "BLOCK", Enabled: true},
			},
		},
		RateLimit: config.RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 200,
			Burst:             400,
			Action:            config.RateLimitActionDrop,
			LogViolations:     true,
			CleanupInterval:   time.Minute,
			MaxTrackedClients: 1000,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Telemetry: config.TelemetryConfig{
			ServiceName:    "glory-hole",
			ServiceVersion: "0.7.x",
			Enabled:        true,
		},
		UpstreamDNSServers:   []string{"1.1.1.1:53", "8.8.8.8:53"},
		Blocklists:           []string{"https://example.com/block.txt"},
		Whitelist:            []string{"allowed.com"},
		AutoUpdateBlocklists: true,
		UpdateInterval:       12 * time.Hour,
	}

	server := New(&Config{
		ListenAddress: ":8080",
		InitialConfig: initial,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	server.handleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp ConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Server.ListenAddress != ":53" {
		t.Errorf("expected listen address :53, got %s", resp.Server.ListenAddress)
	}
	if resp.Cache.MinTTL != (time.Minute).String() {
		t.Errorf("expected min ttl 1m0s, got %s", resp.Cache.MinTTL)
	}
	if len(resp.Policy.Rules) != 1 {
		t.Fatalf("expected 1 policy rule, got %d", len(resp.Policy.Rules))
	}
	if resp.RateLimit.Action != string(config.RateLimitActionDrop) {
		t.Errorf("expected action drop, got %s", resp.RateLimit.Action)
	}
}

func TestHandleGetConfigUnavailable(t *testing.T) {
	server := New(&Config{
		ListenAddress: ":8080",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	server.handleGetConfig(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}
}

func TestHandleStatsTimeSeries(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockStorage{
		timeseries: []*storage.TimeSeriesPoint{
			{
				Timestamp:         now,
				TotalQueries:      100,
				BlockedQueries:    25,
				CachedQueries:     40,
				AvgResponseTimeMs: 4.2,
			},
		},
	}

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/stats/timeseries?period=day&points=10", nil)
	w := httptest.NewRecorder()

	server.handleStatsTimeSeries(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var response TimeSeriesResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Period != "day" {
		t.Errorf("expected period 'day', got %s", response.Period)
	}

	if len(response.Data) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(response.Data))
	}

	if response.Data[0].TotalQueries != 100 {
		t.Errorf("expected total queries 100, got %d", response.Data[0].TotalQueries)
	}
}

func TestHandleQueries(t *testing.T) {
	now := time.Now()
	mock := &mockStorage{
		filtered: []*storage.QueryLog{
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

func TestHandleQueriesAppliesFilters(t *testing.T) {
	mock := &mockStorage{
		filtered: []*storage.QueryLog{},
	}

	server := New(&Config{
		ListenAddress: ":8080",
		Storage:       mock,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/queries?domain=test&type=AAAA&status=blocked&start=2025-01-01T00:00:00Z&end=2025-01-02T00:00:00Z", nil)
	w := httptest.NewRecorder()

	server.handleQueries(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if mock.lastFilter.Domain != "test" {
		t.Errorf("expected domain filter 'test', got %s", mock.lastFilter.Domain)
	}
	if strings.ToUpper(mock.lastFilter.QueryType) != "AAAA" {
		t.Errorf("expected type filter AAAA, got %s", mock.lastFilter.QueryType)
	}
	if mock.lastFilter.Blocked == nil || !*mock.lastFilter.Blocked {
		t.Errorf("expected blocked filter true")
	}
	if mock.lastFilter.Start.IsZero() || mock.lastFilter.End.IsZero() {
		t.Errorf("expected start/end filters to be set")
	}
}

func TestHandleUpdateUpstreams_JSON(t *testing.T) {
	server, configPath := newConfigTestServer(t, func(cfg *config.Config) {
		cfg.UpstreamDNSServers = []string{"1.1.1.1:53"}
	})

	body := `{"servers":["9.9.9.9:53","1.0.0.1:53"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/config/upstreams", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	server.handleUpdateUpstreams(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp ConfigUpdateResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expected := []string{"9.9.9.9:53", "1.0.0.1:53"}
	if len(resp.Config.UpstreamDNSServers) != len(expected) {
		t.Fatalf("expected %d upstreams, got %d", len(expected), len(resp.Config.UpstreamDNSServers))
	}
	for i, serverAddr := range expected {
		if resp.Config.UpstreamDNSServers[i] != serverAddr {
			t.Fatalf("unexpected upstream[%d]: %s", i, resp.Config.UpstreamDNSServers[i])
		}
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	for i, serverAddr := range expected {
		if reloaded.UpstreamDNSServers[i] != serverAddr {
			t.Fatalf("config not persisted, expected %s got %s", serverAddr, reloaded.UpstreamDNSServers[i])
		}
	}
}

func TestHandleUpdateCache_InvalidPayload(t *testing.T) {
	server, _ := newConfigTestServer(t, nil)

	body := `{"enabled":true,"max_entries":1000,"min_ttl":"60s","max_ttl":"30s","negative_ttl":"5m","blocked_ttl":"1s","shard_count":4}`
	req := httptest.NewRequest(http.MethodPut, "/api/config/cache", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	server.handleUpdateCache(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if resp.Message == "" {
		t.Fatal("expected validation error message")
	}
}

func TestHandleUpdateLogging_FormHTMX(t *testing.T) {
	server, configPath := newConfigTestServer(t, func(cfg *config.Config) {
		cfg.Logging.Level = "info"
		cfg.Logging.Format = "text"
		cfg.Logging.Output = "stdout"
		cfg.Logging.FilePath = ""
		cfg.Logging.MaxSize = 100
		cfg.Logging.MaxBackups = 3
		cfg.Logging.MaxAge = 7
	})

	form := url.Values{}
	form.Set("level", "debug")
	form.Set("format", "json")
	form.Set("output", "file")
	form.Set("file_path", "/var/log/glory-hole.log")
	form.Set("add_source", "on")
	form.Set("max_size", "250")
	form.Set("max_backups", "10")
	form.Set("max_age", "30")

	req := httptest.NewRequest(http.MethodPut, "/api/config/logging", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	w := httptest.NewRecorder()
	server.handleUpdateLogging(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("expected HTML content-type, got %s", ct)
	}

	if !strings.Contains(w.Body.String(), "Logging settings updated") {
		t.Fatalf("expected success message in HTMX response, got %s", w.Body.String())
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	if reloaded.Logging.Level != "debug" || reloaded.Logging.Format != "json" || reloaded.Logging.Output != "file" {
		t.Fatalf("logging settings not persisted: %+v", reloaded.Logging)
	}
	if reloaded.Logging.FilePath != "/var/log/glory-hole.log" {
		t.Fatalf("expected file path to update, got %s", reloaded.Logging.FilePath)
	}
	if !reloaded.Logging.AddSource {
		t.Fatal("expected add_source to be true")
	}
	if reloaded.Logging.MaxSize != 250 || reloaded.Logging.MaxBackups != 10 || reloaded.Logging.MaxAge != 30 {
		t.Fatalf("unexpected log retention values: %+v", reloaded.Logging)
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
