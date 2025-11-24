package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/storage"

	"github.com/miekg/dns"
)

func TestNewSharded(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.ShardCount = 16

	cache, err := NewSharded(cfg, logger, nil, cfg.ShardCount)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	if cache == nil {
		t.Fatal("NewSharded() returned nil cache")
	}

	stats := cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("New cache should have 0 entries, got %d", stats.Entries)
	}
}

func TestNewSharded_InvalidConfig(t *testing.T) {
	logger := testLogger(t)

	tests := []struct {
		name       string
		cfg        *config.CacheConfig
		shardCount int
		wantErr    bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "zero max entries",
			cfg: &config.CacheConfig{
				Enabled:    true,
				MaxEntries: 0,
			},
			shardCount: 16,
			wantErr:    true,
		},
		{
			name: "negative max entries",
			cfg: &config.CacheConfig{
				Enabled:    true,
				MaxEntries: -1,
			},
			shardCount: 16,
			wantErr:    true,
		},
		{
			name:       "zero shard count defaults to 64",
			cfg:        testCacheConfig(),
			shardCount: 0,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewSharded(tt.cfg, logger, nil, tt.shardCount)
			if tt.wantErr && err == nil {
				t.Error("NewSharded() should have returned error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("NewSharded() unexpected error: %v", err)
			}
			if cache != nil {
				_ = cache.Close()
			}
		})
	}
}

func TestShardedCache_SetAndGet(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := NewSharded(cfg, logger, nil, 16)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Create query and response
	query := testQuery("example.com", dns.TypeA)
	resp := testResponse("example.com", dns.TypeA, 300)

	// Set in cache
	cache.Set(ctx, query, resp)

	// Get from cache
	cached := cache.Get(ctx, query)
	if cached == nil {
		t.Fatal("Get() returned nil for cached entry")
	}

	// Verify response matches
	if len(cached.Answer) != len(resp.Answer) {
		t.Errorf("Cached response has %d answers, expected %d", len(cached.Answer), len(resp.Answer))
	}

	// Verify stats
	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Sets != 1 {
		t.Errorf("Expected 1 set, got %d", stats.Sets)
	}
	if stats.Entries != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.Entries)
	}
}

func TestShardedCache_Miss(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := NewSharded(cfg, logger, nil, 16)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()
	query := testQuery("example.com", dns.TypeA)

	// Get from empty cache
	cached := cache.Get(ctx, query)
	if cached != nil {
		t.Error("Get() should return nil for cache miss")
	}

	// Verify stats
	stats := cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.Hits != 0 {
		t.Errorf("Expected 0 hits, got %d", stats.Hits)
	}
}

func TestShardedCache_DifferentShards(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	sc, err := NewSharded(cfg, logger, nil, 4)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = sc.Close() }()

	ctx := context.Background()

	// Test that different domains go to different shards
	domains := []string{
		"example1.com",
		"example2.com",
		"example3.com",
		"example4.com",
		"example5.com",
		"example6.com",
		"example7.com",
		"example8.com",
	}

	// Count which shards are used
	usedShards := make(map[int]bool)

	for _, domain := range domains {
		query := testQuery(domain, dns.TypeA)
		key := makeKey(dns.Fqdn(domain), dns.TypeA)

		// Determine which shard this would go to
		shard := sc.getShard(key)

		// Find index of this shard
		for i, s := range sc.shards {
			if s == shard {
				usedShards[i] = true
				break
			}
		}

		// Cache the entry
		resp := testResponse(domain, dns.TypeA, 300)
		sc.Set(ctx, query, resp)
	}

	// With 8 different domains and 4 shards, we should use multiple shards
	if len(usedShards) < 2 {
		t.Errorf("Expected multiple shards to be used, only used %d shards", len(usedShards))
	}

	// Verify all entries are retrievable
	for _, domain := range domains {
		query := testQuery(domain, dns.TypeA)
		cached := sc.Get(ctx, query)
		if cached == nil {
			t.Errorf("Domain %s not cached", domain)
		}
	}
}

func TestShardedCache_TTLExpiration(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MinTTL = 100 * time.Millisecond
	cache, err := NewSharded(cfg, logger, nil, 16)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Create response with very short TTL
	query := testQuery("example.com", dns.TypeA)
	resp := testResponse("example.com", dns.TypeA, 1) // 1 second TTL

	// Set in cache
	cache.Set(ctx, query, resp)

	// Should be cached immediately
	cached := cache.Get(ctx, query)
	if cached == nil {
		t.Fatal("Get() should return cached entry immediately")
	}

	// Wait for expiration (1 second + buffer)
	time.Sleep(1500 * time.Millisecond)

	// Should be expired now
	cached = cache.Get(ctx, query)
	if cached != nil {
		t.Error("Get() should return nil for expired entry")
	}

	// Verify entry was removed
	stats := cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expired entry should be removed, got %d entries", stats.Entries)
	}
}

func TestShardedCache_LRUEviction(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MaxEntries = 40 // 10 entries per shard with 4 shards
	cache, err := NewSharded(cfg, logger, nil, 4)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Add significantly more entries than max to ensure eviction happens in all shards
	// With 100 entries distributed across 4 shards (25 each), and 10 max per shard,
	// we should definitely see evictions
	for i := 1; i <= 100; i++ {
		domain := fmt.Sprintf("example%d.com", i)
		query := testQuery(domain, dns.TypeA)
		resp := testResponse(domain, dns.TypeA, 300)
		cache.Set(ctx, query, resp)
	}

	stats := cache.Stats()
	// Should have evicted some entries
	if stats.Evictions == 0 {
		t.Error("Expected some evictions, got 0")
	}
	// Should not significantly exceed max entries (allowing for some variation due to distribution)
	if stats.Entries > cfg.MaxEntries+4 { // +4 allows for uneven distribution
		t.Errorf("Cache significantly exceeded max entries: %d > %d", stats.Entries, cfg.MaxEntries)
	}
}

func TestShardedCache_Clear(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := NewSharded(cfg, logger, nil, 16)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Add some entries
	for i := 1; i <= 10; i++ {
		domain := fmt.Sprintf("example%d.com", i)
		query := testQuery(domain, dns.TypeA)
		resp := testResponse(domain, dns.TypeA, 300)
		cache.Set(ctx, query, resp)
	}

	stats := cache.Stats()
	if stats.Entries != 10 {
		t.Errorf("Expected 10 entries, got %d", stats.Entries)
	}

	// Clear cache
	cache.Clear()

	// Should have 0 entries
	stats = cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", stats.Entries)
	}

	// Verify entries are gone
	query := testQuery("example1.com", dns.TypeA)
	cached := cache.Get(ctx, query)
	if cached != nil {
		t.Error("Cache should be empty after clear")
	}
}

func TestShardedCache_ConcurrentAccess(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MaxEntries = 10000
	cache, err := NewSharded(cfg, logger, nil, 64)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Run many concurrent operations
	var wg sync.WaitGroup
	numWorkers := 50
	opsPerWorker := 200

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				domain := fmt.Sprintf("example%d-%d.com", workerID, j)
				query := testQuery(domain, dns.TypeA)
				resp := testResponse(domain, dns.TypeA, 300)

				// Mix of operations
				switch j % 3 {
				case 0:
					cache.Set(ctx, query, resp)
				case 1:
					cache.Get(ctx, query)
				case 2:
					cache.SetBlocked(ctx, query, resp, nil)
				}
			}
		}(i)
	}

	// Wait for all goroutines
	wg.Wait()

	// Verify cache is still consistent
	stats := cache.Stats()
	if stats.Entries < 0 {
		t.Error("Cache entries count is negative (race condition)")
	}
	// Note: Hits and Misses are uint64, cannot be negative
}

func TestShardedCache_SetBlocked(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := NewSharded(cfg, logger, nil, 16)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()
	query := testQuery("blocked.example.com", dns.TypeA)

	// Create NXDOMAIN response (blocked)
	resp := new(dns.Msg)
	resp.SetRcode(query, dns.RcodeNameError)

	// Cache the blocked response with trace
	trace := []storage.BlockTraceEntry{
		{
			Stage:  "blocklist",
			Action: "blocked",
			Rule:   "ads.example.com",
			Source: "example.com blocklist",
			Detail: "domain in blocklist",
		},
	}

	// Cache the blocked response
	cache.SetBlocked(ctx, query, resp, trace)

	// Verify it's cached
	cached, cachedTrace := cache.GetWithTrace(ctx, query)
	if cached == nil {
		t.Fatal("SetBlocked() did not cache the response")
	}

	if cached.Rcode != dns.RcodeNameError {
		t.Errorf("Expected Rcode NXDOMAIN, got %d", cached.Rcode)
	}

	// Verify trace is stored and retrieved
	if len(cachedTrace) != 1 {
		t.Errorf("Expected 1 trace entry, got %d", len(cachedTrace))
	} else {
		if cachedTrace[0].Stage != "blocklist" {
			t.Errorf("Expected stage 'blocklist', got '%s'", cachedTrace[0].Stage)
		}
		if cachedTrace[0].Action != "blocked" {
			t.Errorf("Expected action 'blocked', got '%s'", cachedTrace[0].Action)
		}
	}

	// Verify stats
	stats := cache.Stats()
	if stats.Entries != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.Entries)
	}
	if stats.Sets != 1 {
		t.Errorf("Expected 1 set, got %d", stats.Sets)
	}
}

func TestShardedCache_GetWithTrace(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := NewSharded(cfg, logger, nil, 16)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Test regular Set() - should return nil trace
	query1 := testQuery("regular.example.com", dns.TypeA)
	resp1 := testResponse("regular.example.com", dns.TypeA, 300)
	cache.Set(ctx, query1, resp1)

	cached1, trace1 := cache.GetWithTrace(ctx, query1)
	if cached1 == nil {
		t.Error("Expected cached response")
	}
	if trace1 != nil {
		t.Error("Expected nil trace for regular Set()")
	}

	// Test SetBlocked() with trace
	query2 := testQuery("blocked.example.com", dns.TypeA)
	resp2 := new(dns.Msg)
	resp2.SetRcode(query2, dns.RcodeNameError)
	trace2 := []storage.BlockTraceEntry{
		{
			Stage:  "policy",
			Action: "blocked",
			Detail: "policy rule matched",
		},
	}
	cache.SetBlocked(ctx, query2, resp2, trace2)

	cached2, cachedTrace2 := cache.GetWithTrace(ctx, query2)
	if cached2 == nil {
		t.Error("Expected cached blocked response")
	}
	if cachedTrace2 == nil || len(cachedTrace2) != 1 {
		t.Errorf("Expected 1 trace entry, got %d", len(cachedTrace2))
	}
}

func TestShardedCache_StatsAggregation(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MaxEntries = 100
	cache, err := NewSharded(cfg, logger, nil, 4)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Add entries to multiple shards
	for i := 1; i <= 20; i++ {
		domain := fmt.Sprintf("example%d.com", i)
		query := testQuery(domain, dns.TypeA)
		resp := testResponse(domain, dns.TypeA, 300)
		cache.Set(ctx, query, resp)

		// Do a few gets to generate hits
		cache.Get(ctx, query)
		cache.Get(ctx, query)
	}

	// Do some misses
	for i := 100; i <= 105; i++ {
		domain := fmt.Sprintf("example%d.com", i)
		query := testQuery(domain, dns.TypeA)
		cache.Get(ctx, query)
	}

	stats := cache.Stats()

	// Verify aggregated stats
	if stats.Entries != 20 {
		t.Errorf("Expected 20 entries, got %d", stats.Entries)
	}
	if stats.Sets != 20 {
		t.Errorf("Expected 20 sets, got %d", stats.Sets)
	}
	if stats.Hits != 40 {
		t.Errorf("Expected 40 hits (20 domains x 2 gets), got %d", stats.Hits)
	}
	if stats.Misses != 6 {
		t.Errorf("Expected 6 misses, got %d", stats.Misses)
	}

	// Verify hit rate
	expectedHitRate := 40.0 / 46.0 // 40 hits out of 46 total requests
	if stats.HitRate < expectedHitRate-0.01 || stats.HitRate > expectedHitRate+0.01 {
		t.Errorf("Expected hit rate ~%.2f, got %.2f", expectedHitRate, stats.HitRate)
	}
}

func TestShardedCache_CleanupExpiredEntries(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MinTTL = 100 * time.Millisecond
	sc, err := NewSharded(cfg, logger, nil, 4)
	if err != nil {
		t.Fatalf("NewSharded() failed: %v", err)
	}
	defer func() { _ = sc.Close() }()

	ctx := context.Background()

	// Add entries with short TTL
	for i := 1; i <= 10; i++ {
		domain := fmt.Sprintf("example%d.com", i)
		query := testQuery(domain, dns.TypeA)
		resp := testResponse(domain, dns.TypeA, 1) // 1 second TTL
		sc.Set(ctx, query, resp)
	}

	stats := sc.Stats()
	if stats.Entries != 10 {
		t.Errorf("Expected 10 entries, got %d", stats.Entries)
	}

	// Wait for expiration
	time.Sleep(1500 * time.Millisecond)

	// Trigger cleanup
	sc.cleanup()

	// All entries should be removed
	stats = sc.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", stats.Entries)
	}
	if stats.Evictions != 10 {
		t.Errorf("Expected 10 evictions, got %d", stats.Evictions)
	}
}

// Benchmark tests to compare performance

func BenchmarkShardedCache_Set(b *testing.B) {
	logger := &logging.Logger{}
	cfg := testCacheConfig()
	cache, _ := NewSharded(cfg, logger, nil, 64)
	defer func() { _ = cache.Close() }()

	ctx := context.Background()
	query := testQuery("example.com", dns.TypeA)
	resp := testResponse("example.com", dns.TypeA, 300)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(ctx, query, resp)
	}
}

func BenchmarkShardedCache_Get(b *testing.B) {
	logger := &logging.Logger{}
	cfg := testCacheConfig()
	cache, _ := NewSharded(cfg, logger, nil, 64)
	defer func() { _ = cache.Close() }()

	ctx := context.Background()
	query := testQuery("example.com", dns.TypeA)
	resp := testResponse("example.com", dns.TypeA, 300)

	// Pre-populate cache
	cache.Set(ctx, query, resp)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(ctx, query)
	}
}

func BenchmarkShardedCache_Concurrent(b *testing.B) {
	logger := &logging.Logger{}
	cfg := testCacheConfig()
	cfg.MaxEntries = 100000
	cache, _ := NewSharded(cfg, logger, nil, 64)
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Pre-populate with some entries
	for i := 0; i < 100; i++ {
		domain := fmt.Sprintf("example%d.com", i)
		query := testQuery(domain, dns.TypeA)
		resp := testResponse(domain, dns.TypeA, 300)
		cache.Set(ctx, query, resp)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			domain := fmt.Sprintf("example%d.com", i%100)
			query := testQuery(domain, dns.TypeA)
			cache.Get(ctx, query)
			i++
		}
	})
}
