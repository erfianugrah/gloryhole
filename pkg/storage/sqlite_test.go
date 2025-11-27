package storage

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestNewSQLiteStorage(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        ":memory:",
			BusyTimeout: 5000,
			WALMode:     false, // WAL doesn't work well with :memory:
			CacheSize:   1000,
		},
		BufferSize:    100,
		FlushInterval: 1 * time.Second,
		BatchSize:     10,
		RetentionDays: 7,
	}

	storage, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	if storage == nil {
		t.Fatal("expected non-nil storage")
	}

	// Test ping
	ctx := context.Background()
	if err := storage.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestSQLiteStorage_LogQuery(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	query := &QueryLog{
		Timestamp:      time.Now(),
		ClientIP:       "192.168.1.1",
		Domain:         "example.com",
		QueryType:      "A",
		ResponseCode:   0,
		Blocked:        false,
		Cached:         false,
		ResponseTimeMs: 10,
		Upstream:       "1.1.1.1",
	}

	// Log query
	if err := storage.LogQuery(ctx, query); err != nil {
		t.Fatalf("LogQuery() error = %v", err)
	}

	// Give buffer time to flush
	time.Sleep(100 * time.Millisecond)

	// Manually flush by closing and reopening
	// (in a real test we'd wait for flush interval or fill the batch)
}

func TestSQLiteStorage_GetRecentQueries(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Insert some test queries directly into DB (bypass buffer for testing)
	sqlStorage := storage.(*SQLiteStorage)
	for i := 0; i < 5; i++ {
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			time.Now().Add(-time.Duration(i)*time.Minute),
			"192.168.1.1",
			"example.com",
			"A",
			0,
			false,
			false,
			10,
		)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Get recent queries
	queries, err := storage.GetRecentQueries(ctx, 3, 0)
	if err != nil {
		t.Fatalf("GetRecentQueries() error = %v", err)
	}

	if len(queries) != 3 {
		t.Errorf("expected 3 queries, got %d", len(queries))
	}

	// Verify they're ordered by most recent first
	for i := 1; i < len(queries); i++ {
		if queries[i].Timestamp.After(queries[i-1].Timestamp) {
			t.Error("queries not ordered by most recent first")
		}
	}
}

func TestSQLiteStorage_BlockTracePersistence(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	traceJSON := `[{"stage":"policy","action":"block","rule":"kids"}]`

	_, err := sqlStorage.db.Exec(`
		INSERT INTO queries
		(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms, upstream, block_trace)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		time.Now(),
		"192.168.1.50",
		"blocked.example.com",
		"A",
		dns.RcodeNameError,
		true,
		false,
		5,
		"1.1.1.1:53",
		traceJSON,
	)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	queries, err := storage.GetRecentQueries(ctx, 1, 0)
	if err != nil {
		t.Fatalf("GetRecentQueries() error = %v", err)
	}

	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(queries))
	}

	if len(queries[0].BlockTrace) != 1 {
		t.Fatalf("expected block trace entry, got %d", len(queries[0].BlockTrace))
	}

	entry := queries[0].BlockTrace[0]
	if entry.Stage != "policy" || entry.Action != "block" || entry.Rule != "kids" {
		t.Fatalf("unexpected block trace entry: %+v", entry)
	}
}

func TestSQLiteStorage_GetQueriesByDomain(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	// Insert test data for different domains
	domains := []string{"example.com", "test.com", "example.com", "foo.com", "example.com"}
	for _, domain := range domains {
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, time.Now(), "192.168.1.1", domain, "A", 0, false, false, 10)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Get queries for example.com
	queries, err := storage.GetQueriesByDomain(ctx, "example.com", 10)
	if err != nil {
		t.Fatalf("GetQueriesByDomain() error = %v", err)
	}

	if len(queries) != 3 {
		t.Errorf("expected 3 queries for example.com, got %d", len(queries))
	}

	for _, q := range queries {
		if q.Domain != "example.com" {
			t.Errorf("expected domain example.com, got %s", q.Domain)
		}
	}
}

func TestSQLiteStorage_GetQueriesByClientIP(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	// Insert test data for different client IPs
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.1", "10.0.0.1", "192.168.1.1"}
	for _, ip := range ips {
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, time.Now(), ip, "example.com", "A", 0, false, false, 10)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Get queries from 192.168.1.1
	queries, err := storage.GetQueriesByClientIP(ctx, "192.168.1.1", 10)
	if err != nil {
		t.Fatalf("GetQueriesByClientIP() error = %v", err)
	}

	if len(queries) != 3 {
		t.Errorf("expected 3 queries from 192.168.1.1, got %d", len(queries))
	}

	for _, q := range queries {
		if q.ClientIP != "192.168.1.1" {
			t.Errorf("expected client IP 192.168.1.1, got %s", q.ClientIP)
		}
	}
}

func TestSQLiteStorage_GetStatistics(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	// Insert test data
	testData := []struct {
		domain   string
		blocked  bool
		cached   bool
		respTime int64
	}{
		{"example.com", false, false, 10},
		{"blocked.com", true, false, 5},
		{"cached.com", false, true, 2},
		{"test.com", false, false, 15},
		{"blocked2.com", true, false, 3},
	}

	for _, td := range testData {
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, time.Now(), "192.168.1.1", td.domain, "A", 0, td.blocked, td.cached, td.respTime)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Get statistics
	since := time.Now().Add(-1 * time.Hour)
	stats, err := storage.GetStatistics(ctx, since)
	if err != nil {
		t.Fatalf("GetStatistics() error = %v", err)
	}

	if stats.TotalQueries != 5 {
		t.Errorf("expected 5 total queries, got %d", stats.TotalQueries)
	}

	if stats.BlockedQueries != 2 {
		t.Errorf("expected 2 blocked queries, got %d", stats.BlockedQueries)
	}

	if stats.CachedQueries != 1 {
		t.Errorf("expected 1 cached query, got %d", stats.CachedQueries)
	}

	if stats.UniqueDomains != 5 {
		t.Errorf("expected 5 unique domains, got %d", stats.UniqueDomains)
	}

	// Check block rate (2/5 = 40%)
	expectedBlockRate := 40.0
	if stats.BlockRate != expectedBlockRate {
		t.Errorf("expected block rate %.2f%%, got %.2f%%", expectedBlockRate, stats.BlockRate)
	}

	// Check cache hit rate (1/5 = 20%)
	expectedCacheRate := 20.0
	if stats.CacheHitRate != expectedCacheRate {
		t.Errorf("expected cache hit rate %.2f%%, got %.2f%%", expectedCacheRate, stats.CacheHitRate)
	}
}

func TestSQLiteStorage_GetTimeSeriesStats(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	now := time.Now().UTC()
	aligned := now.Truncate(time.Hour)

	testBuckets := []struct {
		timestamp time.Time
		total     int
		blocked   bool
	}{
		{timestamp: aligned.Add(-1 * time.Hour), total: 2, blocked: true},
		{timestamp: aligned, total: 3, blocked: false},
	}

	for _, bucket := range testBuckets {
		for i := 0; i < bucket.total; i++ {
			_, err := sqlStorage.db.Exec(`
				INSERT INTO queries
				(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, bucket.timestamp.Add(time.Duration(i)*time.Minute),
				"10.0.0.1",
				fmt.Sprintf("example-%d.com", i),
				"A",
				0,
				bucket.blocked,
				false,
				10,
			)
			if err != nil {
				t.Fatalf("Failed to insert test data: %v", err)
			}
		}
	}

	points := 4
	result, err := storage.GetTimeSeriesStats(ctx, time.Hour, points)
	if err != nil {
		t.Fatalf("GetTimeSeriesStats() error = %v", err)
	}

	if len(result) != points {
		t.Fatalf("expected %d buckets, got %d", points, len(result))
	}

	if result[len(result)-1].TotalQueries != 3 {
		t.Errorf("expected 3 queries in most recent bucket, got %d", result[len(result)-1].TotalQueries)
	}

	if result[len(result)-2].BlockedQueries != 2 {
		t.Errorf("expected 2 blocked queries in previous bucket, got %d", result[len(result)-2].BlockedQueries)
	}

	if result[0].TotalQueries != 0 {
		t.Errorf("expected zero-filled earliest bucket, got %d", result[0].TotalQueries)
	}
}

func TestSQLiteStorage_GetQueryTypeStats(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	now := time.Now()
	samples := []struct {
		qtype   string
		blocked bool
		cached  bool
		count   int
	}{
		{"A", false, true, 5},
		{"AAAA", true, false, 3},
		{"TXT", false, false, 2},
	}

	for _, sample := range samples {
		for i := 0; i < sample.count; i++ {
			_, err := sqlStorage.db.Exec(`
				INSERT INTO queries
				(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`,
				now.Add(-time.Duration(i)*time.Minute),
				"192.0.2.1",
				fmt.Sprintf("example%d.com", i),
				sample.qtype,
				dns.RcodeSuccess,
				sample.blocked,
				sample.cached,
				5,
			)
			if err != nil {
				t.Fatalf("failed to insert test query: %v", err)
			}
		}
	}

	stats, err := storage.GetQueryTypeStats(ctx, 10, time.Time{})
	if err != nil {
		t.Fatalf("GetQueryTypeStats() error = %v", err)
	}

	if len(stats) != len(samples) {
		t.Fatalf("expected %d query types, got %d", len(samples), len(stats))
	}

	if stats[0].QueryType != "A" || stats[0].Total != 5 || stats[0].Cached != 5 {
		t.Fatalf("unexpected stats for A: %+v", stats[0])
	}

	if stats[1].QueryType != "AAAA" || stats[1].Blocked != 3 {
		t.Fatalf("unexpected stats for AAAA: %+v", stats[1])
	}
}

func TestSQLiteStorage_GetQueryTypeStatsSince(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	now := time.Now().UTC()
	type sample struct {
		qtype string
		at    time.Time
	}

	data := []sample{
		{"A", now.Add(-25 * time.Hour)},
		{"A", now.Add(-2 * time.Hour)},
		{"AAAA", now.Add(-30 * time.Minute)},
	}

	for idx, s := range data {
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, 0, 0, 5)
		`,
			s.at,
			"198.51.100.1",
			fmt.Sprintf("old%d.example", idx),
			s.qtype,
			dns.RcodeSuccess,
		)
		if err != nil {
			t.Fatalf("failed inserting sample %d: %v", idx, err)
		}
	}

	statsAll, err := storage.GetQueryTypeStats(ctx, 10, time.Time{})
	if err != nil {
		t.Fatalf("GetQueryTypeStats(all) error = %v", err)
	}
	if len(statsAll) != 2 {
		t.Fatalf("expected both query types, got %d", len(statsAll))
	}

	since := now.Add(-24 * time.Hour)
	statsRecent, err := storage.GetQueryTypeStats(ctx, 10, since)
	if err != nil {
		t.Fatalf("GetQueryTypeStats(recent) error = %v", err)
	}

	if len(statsRecent) != 2 {
		t.Fatalf("expected 2 query types in last 24h, got %d", len(statsRecent))
	}

	for _, stat := range statsRecent {
		if stat.QueryType == "A" && stat.Total != 1 {
			t.Fatalf("expected only 1 recent A query, got %d", stat.Total)
		}
		if stat.QueryType == "AAAA" && stat.Total != 1 {
			t.Fatalf("expected 1 recent AAAA query, got %d", stat.Total)
		}
	}
}

func TestSQLiteStorage_GetQueriesFiltered(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	now := time.Now().UTC()
	entries := []struct {
		domain  string
		qtype   string
		blocked bool
		cached  bool
	}{
		{"allowed.com", "A", false, false},
		{"blocked.com", "AAAA", true, false},
		{"cached.com", "A", false, true},
		{"test.com", "MX", false, false},
	}

	for _, entry := range entries {
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, now, "10.0.0.1", entry.domain, entry.qtype, 0, entry.blocked, entry.cached, 5)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	trueBool := true
	falseBool := false

	tests := []struct {
		name     string
		filter   QueryFilter
		expected int
	}{
		{
			name:     "domain substring",
			filter:   QueryFilter{Domain: "block"},
			expected: 1,
		},
		{
			name:     "type filter",
			filter:   QueryFilter{QueryType: "a"},
			expected: 2,
		},
		{
			name:     "blocked status",
			filter:   QueryFilter{Blocked: &trueBool},
			expected: 1,
		},
		{
			name:     "allowed status",
			filter:   QueryFilter{Blocked: &falseBool},
			expected: 3,
		},
		{
			name:     "cached status",
			filter:   QueryFilter{Cached: &trueBool},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := storage.GetQueriesFiltered(ctx, tt.filter, 50, 0)
			if err != nil {
				t.Fatalf("GetQueriesFiltered() error = %v", err)
			}
			if len(results) != tt.expected {
				t.Fatalf("expected %d results, got %d", tt.expected, len(results))
			}
		})
	}
}

func TestSQLiteStorage_GetTopDomains(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	// Insert test data with varying query counts
	queries := []struct {
		domain  string
		blocked bool
		count   int
	}{
		{"popular.com", false, 10},
		{"blocked.com", true, 5},
		{"test.com", false, 7},
		{"ads.com", true, 15},
		{"example.com", false, 3},
	}

	for _, q := range queries {
		for i := 0; i < q.count; i++ {
			_, err := sqlStorage.db.Exec(`
				INSERT INTO queries
					(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, time.Now(), "192.168.1.1", q.domain, "A", 0, q.blocked, false, 5)
			if err != nil {
				t.Fatalf("Failed to insert test query: %v", err)
			}
		}
	}

	// Get top non-blocked domains
	topDomains, err := storage.GetTopDomains(ctx, 3, false)
	if err != nil {
		t.Fatalf("GetTopDomains() error = %v", err)
	}

	if len(topDomains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(topDomains))
	}

	// Verify order (most queries first)
	expectedOrder := []string{"popular.com", "test.com", "example.com"}
	for i, expected := range expectedOrder {
		if topDomains[i].Domain != expected {
			t.Errorf("expected domain %s at position %d, got %s", expected, i, topDomains[i].Domain)
		}
	}

	// Get top blocked domains
	topBlocked, err := storage.GetTopDomains(ctx, 2, true)
	if err != nil {
		t.Fatalf("GetTopDomains(blocked) error = %v", err)
	}

	if len(topBlocked) != 2 {
		t.Errorf("expected 2 blocked domains, got %d", len(topBlocked))
	}

	if topBlocked[0].Domain != "ads.com" {
		t.Errorf("expected ads.com to be top blocked domain, got %s", topBlocked[0].Domain)
	}
}

func TestSQLiteStorage_GetBlockedCount(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	// Insert test data
	now := time.Now()
	for i := 0; i < 10; i++ {
		blocked := i%3 == 0 // Every third query is blocked
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, now, "192.168.1.1", "example.com", "A", 0, blocked, false, 10)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Get blocked count
	since := now.Add(-1 * time.Hour)
	count, err := storage.GetBlockedCount(ctx, since)
	if err != nil {
		t.Fatalf("GetBlockedCount() error = %v", err)
	}

	expectedCount := int64(4) // 0, 3, 6, 9
	if count != expectedCount {
		t.Errorf("expected %d blocked queries, got %d", expectedCount, count)
	}
}

func TestSQLiteStorage_GetQueryCount(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	// Insert test data
	now := time.Now()
	for i := 0; i < 15; i++ {
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, now, "192.168.1.1", "example.com", "A", 0, false, false, 10)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Get query count
	since := now.Add(-1 * time.Hour)
	count, err := storage.GetQueryCount(ctx, since)
	if err != nil {
		t.Fatalf("GetQueryCount() error = %v", err)
	}

	if count != 15 {
		t.Errorf("expected 15 queries, got %d", count)
	}
}

func TestSQLiteStorage_Cleanup(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	// Insert test data with varying timestamps
	now := time.Now()
	for i := 0; i < 10; i++ {
		timestamp := now.Add(-time.Duration(i) * 24 * time.Hour)
		_, err := sqlStorage.db.Exec(`
			INSERT INTO queries
			(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, timestamp, "192.168.1.1", "example.com", "A", 0, false, false, 10)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Clean up queries older than 5 days
	cutoff := now.Add(-5 * 24 * time.Hour)
	if err := storage.Cleanup(ctx, cutoff); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify only recent queries remain
	var count int64
	err := sqlStorage.db.QueryRow("SELECT COUNT(*) FROM queries").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count queries: %v", err)
	}

	// Should have 6 queries (0-5 days old)
	expectedCount := int64(6)
	if count != expectedCount {
		t.Errorf("expected %d queries after cleanup, got %d", expectedCount, count)
	}
}

func TestSQLiteStorage_Reset(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	now := time.Now()
	_, err := sqlStorage.db.Exec(`
		INSERT INTO queries (timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
		VALUES (?, ?, ?, 'A', 0, 0, 0, 5)
	`, now, "10.0.0.2", "nuke.test")
	if err != nil {
		t.Fatalf("failed to insert query data: %v", err)
	}

	_, err = sqlStorage.db.Exec(`
		INSERT INTO statistics (hour, total_queries)
		VALUES (?, 42)
	`, now.Truncate(time.Hour))
	if err != nil {
		t.Fatalf("failed to insert statistics data: %v", err)
	}

	_, err = sqlStorage.db.Exec(`
		INSERT INTO domain_stats (domain, query_count, last_queried, first_queried, blocked)
		VALUES ('nuke.test', 1, ?, ?, 0)
	`, now, now)
	if err != nil {
		t.Fatalf("failed to insert domain stats: %v", err)
	}

	_, err = sqlStorage.db.Exec(`
		INSERT INTO client_groups (name, description)
		VALUES ('testers', 'Test Group')
	`)
	if err != nil {
		t.Fatalf("failed to insert client group: %v", err)
	}

	_, err = sqlStorage.db.Exec(`
		INSERT INTO client_profiles (client_ip, display_name, group_name)
		VALUES ('10.0.0.2', 'Client', 'testers')
	`)
	if err != nil {
		t.Fatalf("failed to insert client profile: %v", err)
	}

	if err := storage.Reset(ctx); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	tables := map[string]string{
		"queries":         "query",
		"statistics":      "stat",
		"domain_stats":    "domain stat",
		"client_groups":   "client group",
		"client_profiles": "client profile",
	}

	for table, label := range tables {
		var count int64
		if err := sqlStorage.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("failed counting %s rows: %v", table, err)
		}
		if count != 0 {
			t.Errorf("expected %s table to be empty after reset, got %d rows", label, count)
		}
	}
}

func TestSQLiteStorage_Close(t *testing.T) {
	storage, _ := setupTestStorage(t)

	// Close storage
	if err := storage.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify further operations fail
	ctx := context.Background()
	if err := storage.LogQuery(ctx, &QueryLog{}); err != ErrClosed {
		t.Errorf("expected ErrClosed after Close(), got %v", err)
	}
}

func TestSQLiteStorage_ClientSummaries(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	sqlStorage := storage.(*SQLiteStorage)

	now := time.Now()
	entries := []struct {
		ip      string
		blocked bool
		rcode   int
		domain  string
	}{
		{"192.168.1.10", true, dns.RcodeNameError, "blocked.local"},
		{"192.168.1.10", false, dns.RcodeSuccess, "allowed.local"},
		{"192.168.1.20", false, dns.RcodeNameError, "nx.local"},
	}

	for _, e := range entries {
		if _, err := sqlStorage.db.Exec(`
			INSERT INTO queries
				(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, now, e.ip, e.domain, "A", e.rcode, e.blocked, false, 5); err != nil {
			t.Fatalf("failed to insert query: %v", err)
		}
	}

	if _, err := sqlStorage.db.Exec(`INSERT INTO client_groups (name, description, color) VALUES ('Kids', 'Kid devices', '#f472b6')`); err != nil {
		t.Fatalf("failed to insert client group: %v", err)
	}

	if err := storage.UpdateClientProfile(ctx, &ClientProfile{
		ClientIP:    "192.168.1.10",
		DisplayName: "Living Room Apple TV",
		GroupName:   "Kids",
	}); err != nil {
		t.Fatalf("UpdateClientProfile() error = %v", err)
	}

	summaries, err := storage.GetClientSummaries(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetClientSummaries() error = %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	for _, summary := range summaries {
		switch summary.ClientIP {
		case "192.168.1.10":
			if summary.DisplayName != "Living Room Apple TV" {
				t.Errorf("unexpected display name %s", summary.DisplayName)
			}
			if summary.LastSeen.IsZero() || summary.FirstSeen.IsZero() {
				t.Errorf("expected timestamps for %s, got zero values", summary.ClientIP)
			}
			if summary.GroupName != "Kids" {
				t.Errorf("expected group Kids, got %s", summary.GroupName)
			}
			if summary.BlockedQueries != 1 {
				t.Errorf("expected 1 blocked query, got %d", summary.BlockedQueries)
			}
		case "192.168.1.20":
			if summary.LastSeen.IsZero() || summary.FirstSeen.IsZero() {
				t.Errorf("expected timestamps for %s, got zero values", summary.ClientIP)
			}
			if summary.NXDomainCount != 1 {
				t.Errorf("expected 1 NXDOMAIN count, got %d", summary.NXDomainCount)
			}
		default:
			t.Fatalf("unexpected client summary for %s", summary.ClientIP)
		}
	}
}

func TestSQLiteStorage_Persistence(t *testing.T) {
	// Create a temporary database file
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	_ = tmpfile.Close()

	cfg := &Config{
		Enabled: true,
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        tmpfile.Name(),
			BusyTimeout: 5000,
			WALMode:     true,
			CacheSize:   1000,
		},
		BufferSize:    100,
		FlushInterval: 100 * time.Millisecond,
		BatchSize:     10,
		RetentionDays: 7,
	}

	// Create storage and insert data
	storage1, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}

	sqlStorage1 := storage1.(*SQLiteStorage)
	_, err = sqlStorage1.db.Exec(`
		INSERT INTO queries
		(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, time.Now(), "192.168.1.1", "example.com", "A", 0, false, false, 10)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	_ = storage1.Close()

	// Reopen storage and verify data persisted
	storage2, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		t.Fatalf("NewSQLiteStorage() error = %v", err)
	}
	defer func() { _ = storage2.Close() }()

	ctx := context.Background()
	queries, err := storage2.GetRecentQueries(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetRecentQueries() error = %v", err)
	}

	if len(queries) != 1 {
		t.Errorf("expected 1 persisted query, got %d", len(queries))
	}

	if len(queries) > 0 && queries[0].Domain != "example.com" {
		t.Errorf("expected domain example.com, got %s", queries[0].Domain)
	}
}

// Helper function to setup test storage
func setupTestStorage(t *testing.T) (Storage, func()) {
	cfg := &Config{
		Enabled: true,
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        ":memory:",
			BusyTimeout: 5000,
			WALMode:     false,
			CacheSize:   1000,
		},
		BufferSize:    100,
		FlushInterval: 1 * time.Second,
		BatchSize:     10,
		RetentionDays: 7,
	}

	storage, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		t.Fatalf("setupTestStorage() error = %v", err)
	}

	cleanup := func() {
		_ = storage.Close()
	}

	return storage, cleanup
}
