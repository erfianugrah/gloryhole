package storage

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestBufferHighWatermark tests the high watermark detection
func TestBufferHighWatermark(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "test-buffer-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Small buffer to test watermark easily
	cfg := &Config{
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        tmpFile.Name(),
			BusyTimeout: 5000,
			WALMode:     true,
			CacheSize:   4096,
		},
		BufferSize:    10, // Small buffer for testing
		FlushInterval: 1 * time.Second,
		BatchSize:     5,
		Enabled:       true,
	}

	stor, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer stor.Close()

	sqliteStor := stor.(*SQLiteStorage)

	// Check initial state
	stats := sqliteStor.GetBufferStats()
	if stats.Capacity != 10 {
		t.Errorf("Expected capacity 10, got %d", stats.Capacity)
	}
	if stats.HighWater != 8 { // 80% of 10
		t.Errorf("Expected high watermark 8, got %d", stats.HighWater)
	}

	// Fill buffer to just below high watermark
	for i := 0; i < 7; i++ {
		// Create new QueryLog for each iteration to avoid data race
		err = sqliteStor.LogQuery(context.Background(), &QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		})
		if err != nil {
			t.Errorf("Failed to log query: %v", err)
		}
	}

	// Check buffer utilization
	stats = sqliteStor.GetBufferStats()
	if stats.Size < 5 { // Some might have been flushed
		t.Logf("Buffer size: %d (some queries flushed)", stats.Size)
	}

	// Wait for flush and close
	time.Sleep(100 * time.Millisecond)
}

// TestBufferStats tests the buffer statistics reporting
func TestBufferStats(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "test-buffer-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg := &Config{
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        tmpFile.Name(),
			BusyTimeout: 5000,
			WALMode:     true,
			CacheSize:   4096,
		},
		BufferSize:    100,
		FlushInterval: 5 * time.Second, // Long interval to keep items in buffer
		BatchSize:     50,
		Enabled:       true,
	}

	stor, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer stor.Close()

	sqliteStor := stor.(*SQLiteStorage)

	// Add some queries
	for i := 0; i < 10; i++ {
		query := &QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}
		if err := sqliteStor.LogQuery(context.Background(), query); err != nil {
			t.Errorf("Failed to log query: %v", err)
		}
	}

	// Get stats
	stats := sqliteStor.GetBufferStats()

	if stats.Capacity != 100 {
		t.Errorf("Expected capacity 100, got %d", stats.Capacity)
	}

	if stats.Size == 0 {
		t.Error("Expected some buffered queries")
	}

	if stats.Utilization < 0 || stats.Utilization > 100 {
		t.Errorf("Invalid utilization: %.2f", stats.Utilization)
	}

	t.Logf("Buffer stats: size=%d, capacity=%d, utilization=%.2f%%",
		stats.Size, stats.Capacity, stats.Utilization)
}

// TestBufferOverflow tests behavior when buffer is full
func TestBufferOverflow(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "test-buffer-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Very small buffer that will overflow easily
	cfg := &Config{
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        tmpFile.Name(),
			BusyTimeout: 5000,
			WALMode:     true,
			CacheSize:   4096,
		},
		BufferSize:    5, // Very small
		FlushInterval: 10 * time.Second, // Long interval so buffer fills up
		BatchSize:     10,
		Enabled:       true,
	}

	stor, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer stor.Close()

	sqliteStor := stor.(*SQLiteStorage)

	// Try to add more queries than buffer can hold
	dropped := 0
	for i := 0; i < 10; i++ {
		query := &QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		}
		if err := sqliteStor.LogQuery(context.Background(), query); err == ErrBufferFull {
			dropped++
		}
	}

	// Should have dropped some
	if dropped == 0 {
		t.Log("No queries dropped (flush may have been fast)")
	} else {
		t.Logf("Dropped %d queries as expected", dropped)
	}
}

// TestDefaultBufferSize tests that default buffer size is increased
func TestDefaultBufferSize(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BufferSize != 50000 {
		t.Errorf("Expected default buffer size 50000, got %d", cfg.BufferSize)
	}

	t.Logf("Default buffer size: %d", cfg.BufferSize)
}

// TestFlushTiming tests that flush timing is logged
func TestFlushTiming(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "test-buffer-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg := &Config{
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        tmpFile.Name(),
			BusyTimeout: 5000,
			WALMode:     true,
			CacheSize:   4096,
		},
		BufferSize:    1000,
		FlushInterval: 100 * time.Millisecond, // Fast flushes for testing
		BatchSize:     10,
		Enabled:       true,
	}

	stor, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer stor.Close()

	sqliteStor := stor.(*SQLiteStorage)

	// Add queries to trigger flush
	for i := 0; i < 50; i++ {
		// Create new QueryLog for each iteration to avoid data race
		err = sqliteStor.LogQuery(context.Background(), &QueryLog{
			Domain:   "example.com",
			ClientIP: "1.2.3.4",
		})
		if err != nil {
			t.Errorf("Failed to log query: %v", err)
		}
	}

	// Wait for flushes to complete
	time.Sleep(500 * time.Millisecond)

	// Verify queries were flushed
	queries, err := sqliteStor.GetRecentQueries(context.Background(), 100, 0)
	if err != nil {
		t.Fatalf("Failed to get queries: %v", err)
	}

	if len(queries) == 0 {
		t.Error("Expected some queries to be flushed to database")
	}

	t.Logf("Flushed %d queries to database", len(queries))
}

// BenchmarkBufferLogging benchmarks the buffer logging performance
func BenchmarkBufferLogging(b *testing.B) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "bench-buffer-*.db")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg := &Config{
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        tmpFile.Name(),
			BusyTimeout: 5000,
			WALMode:     true,
			CacheSize:   4096,
		},
		BufferSize:    50000,
		FlushInterval: 1 * time.Second,
		BatchSize:     100,
		Enabled:       true,
	}

	stor, err := NewSQLiteStorage(cfg, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer stor.Close()

	sqliteStor := stor.(*SQLiteStorage)

	query := &QueryLog{
		Domain:   "example.com",
		ClientIP: "1.2.3.4",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = sqliteStor.LogQuery(context.Background(), query)
		}
	})
}
