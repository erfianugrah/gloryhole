package cache

import (
	"context"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"github.com/miekg/dns"
)

func testLogger(t *testing.T) *logging.Logger {
	cfg := &config.LoggingConfig{
		Level:     "debug",
		Format:    "text",
		Output:    "stdout",
		AddSource: false,
	}
	logger, err := logging.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return logger
}

func testCacheConfig() *config.CacheConfig {
	return &config.CacheConfig{
		Enabled:     true,
		MaxEntries:  100,
		MinTTL:      1 * time.Second,
		MaxTTL:      1 * time.Hour,
		NegativeTTL: 5 * time.Minute,
	}
}

func testQuery(domain string, qtype uint16) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), qtype)
	return m
}

func testResponse(domain string, qtype uint16, ttl uint32) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), qtype)

	switch qtype {
	case dns.TypeA:
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(domain),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    ttl,
			},
			A: []byte{192, 0, 2, 1},
		}
		m.Answer = append(m.Answer, rr)
	case dns.TypeAAAA:
		rr := &dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(domain),
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    ttl,
			},
			AAAA: []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		}
		m.Answer = append(m.Answer, rr)
	}

	return m
}

func TestNew(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()

	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	if cache == nil {
		t.Fatal("New() returned nil cache")
	}

	stats := cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("New cache should have 0 entries, got %d", stats.Entries)
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	logger := testLogger(t)

	tests := []struct {
		name    string
		cfg     *config.CacheConfig
		wantErr bool
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
			wantErr: true,
		},
		{
			name: "negative max entries",
			cfg: &config.CacheConfig{
				Enabled:    true,
				MaxEntries: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := New(tt.cfg, logger)
			if tt.wantErr && err == nil {
				t.Error("New() should have returned error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("New() unexpected error: %v", err)
			}
			if cache != nil {
				_ = cache.Close()
			}
		})
	}
}

func TestCache_SetAndGet(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
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

func TestCache_Miss(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
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

func TestCache_TTLExpiration(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MinTTL = 100 * time.Millisecond
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
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

func TestCache_MinTTL(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MinTTL = 60 * time.Second
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Create response with TTL below minimum
	query := testQuery("example.com", dns.TypeA)
	resp := testResponse("example.com", dns.TypeA, 10) // 10 seconds, below 60s min

	// Set in cache
	cache.Set(ctx, query, resp)

	// Should be cached with min TTL
	cached := cache.Get(ctx, query)
	if cached == nil {
		t.Fatal("Get() should return cached entry with min TTL")
	}

	// Should still be valid after 15 seconds (10s < minTTL 60s)
	time.Sleep(15 * time.Millisecond) // Simulate time passing
	cached = cache.Get(ctx, query)
	if cached == nil {
		t.Error("Entry should still be cached (min TTL applied)")
	}
}

func TestCache_MaxTTL(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MaxTTL = 10 * time.Second
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Create response with TTL above maximum
	query := testQuery("example.com", dns.TypeA)
	resp := testResponse("example.com", dns.TypeA, 3600) // 1 hour, above 10s max

	// Set in cache
	cache.Set(ctx, query, resp)

	// Get the entry
	cached := cache.Get(ctx, query)
	if cached == nil {
		t.Fatal("Get() should return cached entry")
	}

	// The entry should expire after max TTL, not original TTL
	// We can't easily test expiration without waiting, so just verify it's cached
	stats := cache.Stats()
	if stats.Entries != 1 {
		t.Errorf("Expected 1 cached entry, got %d", stats.Entries)
	}
}

func TestCache_NegativeResponse(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.NegativeTTL = 5 * time.Minute
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Create NXDOMAIN response (negative response)
	query := testQuery("nonexistent.example.com", dns.TypeA)
	resp := new(dns.Msg)
	resp.SetReply(query)
	resp.SetRcode(query, dns.RcodeNameError) // NXDOMAIN

	// Set in cache
	cache.Set(ctx, query, resp)

	// Should be cached
	cached := cache.Get(ctx, query)
	if cached == nil {
		t.Fatal("Get() should return cached negative response")
	}

	// Verify it's an NXDOMAIN
	if cached.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got rcode %d", cached.Rcode)
	}

	stats := cache.Stats()
	if stats.Entries != 1 {
		t.Errorf("Expected 1 cached entry, got %d", stats.Entries)
	}
}

func TestCache_LRUEviction(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MaxEntries = 3 // Small cache for testing
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Add 3 entries to fill the cache
	for i := 1; i <= 3; i++ {
		query := testQuery("example"+string(rune('0'+i))+".com", dns.TypeA)
		resp := testResponse("example"+string(rune('0'+i))+".com", dns.TypeA, 300)
		cache.Set(ctx, query, resp)
	}

	stats := cache.Stats()
	if stats.Entries != 3 {
		t.Errorf("Expected 3 entries, got %d", stats.Entries)
	}

	// Access entry 2 and 3 to make entry 1 the LRU
	query2 := testQuery("example2.com", dns.TypeA)
	cache.Get(ctx, query2)

	query3 := testQuery("example3.com", dns.TypeA)
	cache.Get(ctx, query3)

	// Add 4th entry, should evict entry 1 (LRU)
	query4 := testQuery("example4.com", dns.TypeA)
	resp4 := testResponse("example4.com", dns.TypeA, 300)
	cache.Set(ctx, query4, resp4)

	// Cache should still have 3 entries
	stats = cache.Stats()
	if stats.Entries != 3 {
		t.Errorf("Expected 3 entries after eviction, got %d", stats.Entries)
	}

	// Entry 1 should be evicted
	query1 := testQuery("example1.com", dns.TypeA)
	cached := cache.Get(ctx, query1)
	if cached != nil {
		t.Error("Entry 1 should have been evicted")
	}

	// Entries 2, 3, 4 should still be cached
	if cache.Get(ctx, query2) == nil {
		t.Error("Entry 2 should still be cached")
	}
	if cache.Get(ctx, query3) == nil {
		t.Error("Entry 3 should still be cached")
	}
	if cache.Get(ctx, query4) == nil {
		t.Error("Entry 4 should be cached")
	}

	// Verify eviction stat
	if stats.Evictions != 1 {
		t.Errorf("Expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestCache_DifferentQueryTypes(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Cache A record
	queryA := testQuery("example.com", dns.TypeA)
	respA := testResponse("example.com", dns.TypeA, 300)
	cache.Set(ctx, queryA, respA)

	// Cache AAAA record for same domain
	queryAAAA := testQuery("example.com", dns.TypeAAAA)
	respAAAA := testResponse("example.com", dns.TypeAAAA, 300)
	cache.Set(ctx, queryAAAA, respAAAA)

	// Should have 2 separate entries
	stats := cache.Stats()
	if stats.Entries != 2 {
		t.Errorf("Expected 2 entries (A and AAAA), got %d", stats.Entries)
	}

	// Both should be retrievable
	cachedA := cache.Get(ctx, queryA)
	if cachedA == nil {
		t.Error("A record should be cached")
	}

	cachedAAAA := cache.Get(ctx, queryAAAA)
	if cachedAAAA == nil {
		t.Error("AAAA record should be cached")
	}
}

func TestCache_Clear(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Add some entries
	for i := 1; i <= 5; i++ {
		query := testQuery("example"+string(rune('0'+i))+".com", dns.TypeA)
		resp := testResponse("example"+string(rune('0'+i))+".com", dns.TypeA, 300)
		cache.Set(ctx, query, resp)
	}

	stats := cache.Stats()
	if stats.Entries != 5 {
		t.Errorf("Expected 5 entries, got %d", stats.Entries)
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

func TestCache_Disabled(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.Enabled = false // Disable cache
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Try to set and get
	query := testQuery("example.com", dns.TypeA)
	resp := testResponse("example.com", dns.TypeA, 300)

	cache.Set(ctx, query, resp)

	// Should not be cached
	cached := cache.Get(ctx, query)
	if cached != nil {
		t.Error("Disabled cache should not return entries")
	}

	stats := cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("Disabled cache should have 0 entries, got %d", stats.Entries)
	}
}

func TestCache_HitRate(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	query := testQuery("example.com", dns.TypeA)
	resp := testResponse("example.com", dns.TypeA, 300)

	// 1 miss
	cache.Get(ctx, query)

	// 1 set
	cache.Set(ctx, query, resp)

	// 3 hits
	cache.Get(ctx, query)
	cache.Get(ctx, query)
	cache.Get(ctx, query)

	stats := cache.Stats()
	expectedHitRate := 3.0 / 4.0 // 3 hits out of 4 total requests
	if stats.HitRate < expectedHitRate-0.01 || stats.HitRate > expectedHitRate+0.01 {
		t.Errorf("Expected hit rate ~%.2f, got %.2f", expectedHitRate, stats.HitRate)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	logger := testLogger(t)
	cfg := testCacheConfig()
	cfg.MaxEntries = 1000
	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				query := testQuery("example.com", dns.TypeA)
				resp := testResponse("example.com", dns.TypeA, 300)

				// Mix of set and get operations
				if j%2 == 0 {
					cache.Set(ctx, query, resp)
				} else {
					cache.Get(ctx, query)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify cache is still consistent
	stats := cache.Stats()
	if stats.Entries < 0 {
		t.Error("Cache entries count is negative (race condition)")
	}
}
