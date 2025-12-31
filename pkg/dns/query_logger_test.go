package dns

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"glory-hole/pkg/storage"
)

// mockStorage implements storage.Storage for testing
type mockStorage struct {
	logs      []*storage.QueryLog
	mu        sync.Mutex
	logCount  atomic.Int64
	failCount int // Fail first N log attempts
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		logs: make([]*storage.QueryLog, 0),
	}
}

func (m *mockStorage) LogQuery(ctx context.Context, query *storage.QueryLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate failures for testing
	if m.failCount > 0 {
		m.failCount--
		return errors.New("simulated storage error")
	}

	m.logs = append(m.logs, query)
	m.logCount.Add(1)
	return nil
}

func (m *mockStorage) GetLogs() []*storage.QueryLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*storage.QueryLog{}, m.logs...)
}

func (m *mockStorage) Count() int64 {
	return m.logCount.Load()
}

// Stub implementations for other Storage interface methods
func (m *mockStorage) GetRecentQueries(ctx context.Context, limit, offset int) ([]*storage.QueryLog, error) {
	return nil, nil
}
func (m *mockStorage) GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*storage.QueryLog, error) {
	return nil, nil
}
func (m *mockStorage) GetQueriesByClientIP(ctx context.Context, clientIP string, limit int) ([]*storage.QueryLog, error) {
	return nil, nil
}
func (m *mockStorage) GetQueriesFiltered(ctx context.Context, filter storage.QueryFilter, limit, offset int) ([]*storage.QueryLog, error) {
	return nil, nil
}
func (m *mockStorage) GetStatistics(ctx context.Context, since time.Time) (*storage.Statistics, error) {
	return nil, nil
}
func (m *mockStorage) GetTopDomains(ctx context.Context, limit int, blocked bool, since time.Time) ([]*storage.DomainStats, error) {
	return nil, nil
}
func (m *mockStorage) GetBlockedCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}
func (m *mockStorage) GetQueryCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}
func (m *mockStorage) GetTimeSeriesStats(ctx context.Context, bucket time.Duration, points int) ([]*storage.TimeSeriesPoint, error) {
	return nil, nil
}
func (m *mockStorage) GetQueryTypeStats(ctx context.Context, limit int, since time.Time) ([]*storage.QueryTypeStats, error) {
	return nil, nil
}
func (m *mockStorage) GetTraceStatistics(ctx context.Context, since time.Time) (*storage.TraceStatistics, error) {
	return nil, nil
}
func (m *mockStorage) GetQueriesWithTraceFilter(ctx context.Context, filter storage.TraceFilter, limit, offset int) ([]*storage.QueryLog, error) {
	return nil, nil
}
func (m *mockStorage) GetClientSummaries(ctx context.Context, limit, offset int) ([]*storage.ClientSummary, error) {
	return nil, nil
}
func (m *mockStorage) UpdateClientProfile(ctx context.Context, profile *storage.ClientProfile) error {
	return nil
}
func (m *mockStorage) GetClientGroups(ctx context.Context) ([]*storage.ClientGroup, error) {
	return nil, nil
}
func (m *mockStorage) UpsertClientGroup(ctx context.Context, group *storage.ClientGroup) error {
	return nil
}
func (m *mockStorage) DeleteClientGroup(ctx context.Context, name string) error {
	return nil
}
func (m *mockStorage) Cleanup(ctx context.Context, olderThan time.Time) error {
	return nil
}
func (m *mockStorage) Reset(ctx context.Context) error {
	return nil
}
func (m *mockStorage) Close() error {
	return nil
}
func (m *mockStorage) Ping(ctx context.Context) error {
	return nil
}

func TestQueryLogger_BasicOperation(t *testing.T) {
	stor := newMockStorage()
	ql := NewQueryLogger(stor, nil, 100, 2)
	defer ql.Close()

	// Log some queries
	for i := 0; i < 10; i++ {
		entry := &storage.QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}
		if err := ql.LogAsync(entry); err != nil {
			t.Fatalf("Failed to log query: %v", err)
		}
	}

	// Wait for workers to process
	time.Sleep(100 * time.Millisecond)

	// Close and drain
	if err := ql.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Verify all queries logged
	if stor.Count() != 10 {
		t.Errorf("Expected 10 queries logged, got %d", stor.Count())
	}
}

func TestQueryLogger_BufferFull(t *testing.T) {
	stor := newMockStorage()
	// Small buffer that will fill up
	ql := NewQueryLogger(stor, nil, 5, 1)
	defer ql.Close()

	// Log more than buffer capacity
	dropped := 0
	for i := 0; i < 10; i++ {
		entry := &storage.QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}
		if err := ql.LogAsync(entry); err != nil {
			dropped++
		}
	}

	// Should have dropped some
	if dropped == 0 {
		t.Error("Expected some queries to be dropped when buffer full")
	}

	// Check stats
	_, droppedCount := ql.Stats()
	if droppedCount == 0 {
		t.Error("Expected dropped count > 0")
	}
}

func TestQueryLogger_ConcurrentLogging(t *testing.T) {
	stor := newMockStorage()
	ql := NewQueryLogger(stor, nil, 1000, 4)
	defer ql.Close()

	// Concurrent logging from multiple goroutines
	var wg sync.WaitGroup
	numGoroutines := 10
	queriesPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < queriesPerGoroutine; j++ {
				entry := &storage.QueryLog{
					Domain:   "example.com",
					ClientIP: "1.2.3.4",
				}
				_ = ql.LogAsync(entry)
			}
		}(i)
	}

	wg.Wait()

	// Wait for workers to process
	time.Sleep(200 * time.Millisecond)

	// Close and drain
	if err := ql.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Verify all queries logged (allowing for some drops due to buffer size)
	expected := int64(numGoroutines * queriesPerGoroutine)
	actual := stor.Count()

	// Should get most queries (at least 90%)
	if actual < (expected * 90 / 100) {
		t.Errorf("Expected at least %d queries, got %d", expected*90/100, actual)
	}
}

func TestQueryLogger_GracefulShutdown(t *testing.T) {
	stor := newMockStorage()
	ql := NewQueryLogger(stor, nil, 100, 2)

	// Log some queries
	for i := 0; i < 20; i++ {
		entry := &storage.QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}
		_ = ql.LogAsync(entry)
	}

	// Close immediately (should drain remaining entries)
	if err := ql.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// All queries should be processed during drain
	if stor.Count() != 20 {
		t.Errorf("Expected 20 queries after graceful shutdown, got %d", stor.Count())
	}
}

func TestQueryLogger_StorageError(t *testing.T) {
	stor := newMockStorage()
	stor.failCount = 5 // First 5 attempts will fail

	ql := NewQueryLogger(stor, nil, 100, 2)
	defer ql.Close()

	// Log queries (first 5 will fail in storage)
	for i := 0; i < 10; i++ {
		entry := &storage.QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}
		_ = ql.LogAsync(entry)
	}

	// Wait for workers to process
	time.Sleep(100 * time.Millisecond)

	// Close and drain
	if err := ql.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Only 5 should succeed (last 5)
	if stor.Count() != 5 {
		t.Errorf("Expected 5 successful queries, got %d", stor.Count())
	}
}

func TestQueryLogger_Stats(t *testing.T) {
	stor := newMockStorage()
	ql := NewQueryLogger(stor, nil, 10, 2)
	defer ql.Close()

	// Buffer capacity
	if ql.BufferCapacity() != 10 {
		t.Errorf("Expected buffer capacity 10, got %d", ql.BufferCapacity())
	}

	// Log some queries
	for i := 0; i < 5; i++ {
		entry := &storage.QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}
		_ = ql.LogAsync(entry)
	}

	// Check buffered count
	buffered, dropped := ql.Stats()
	if buffered == 0 {
		t.Error("Expected some buffered entries")
	}
	if dropped != 0 {
		t.Error("Expected no dropped entries")
	}

	// Try to overflow buffer
	for i := 0; i < 10; i++ {
		entry := &storage.QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}
		_ = ql.LogAsync(entry)
	}

	// Should have some drops now
	_, dropped = ql.Stats()
	if dropped == 0 {
		t.Error("Expected some dropped entries after overflow")
	}
}

func BenchmarkQueryLogger_LogAsync(b *testing.B) {
	stor := newMockStorage()
	ql := NewQueryLogger(stor, nil, 50000, 8)
	defer ql.Close()

	entry := &storage.QueryLog{
		Domain:   "example.com",
		ClientIP: "1.2.3.4",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = ql.LogAsync(entry)
		}
	})
}

func BenchmarkQueryLogger_vs_Goroutine(b *testing.B) {
	stor := newMockStorage()

	b.Run("WorkerPool", func(b *testing.B) {
		ql := NewQueryLogger(stor, nil, 50000, 8)
		defer ql.Close()

		entry := &storage.QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = ql.LogAsync(entry)
		}
	})

	b.Run("GoroutineSpawn", func(b *testing.B) {
		entry := &storage.QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()
				_ = stor.LogQuery(ctx, entry)
			}()
		}
	})
}
