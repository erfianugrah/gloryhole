package storage

import (
	"context"
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

	storage, err := NewSQLiteStorage(cfg)
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
		// Use domain_stats directly for faster testing
		_, err := sqlStorage.db.Exec(`
			INSERT INTO domain_stats (domain, query_count, last_queried, first_queried, blocked)
			VALUES (?, ?, ?, ?, ?)
		`, q.domain, q.count, time.Now(), time.Now(), q.blocked)
		if err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
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
	storage1, err := NewSQLiteStorage(cfg)
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
	storage2, err := NewSQLiteStorage(cfg)
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

	storage, err := NewSQLiteStorage(cfg)
	if err != nil {
		t.Fatalf("setupTestStorage() error = %v", err)
	}

	cleanup := func() {
		_ = storage.Close()
	}

	return storage, cleanup
}
