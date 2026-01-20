package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/storage"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
)

// fnv1aHashString computes FNV-1a hash of a string without allocations.
// This is significantly faster than using hash/fnv which requires []byte conversion.
func fnv1aHashString(s string) uint32 {
	const offset32 = 2166136261
	const prime32 = 16777619

	hash := uint32(offset32)
	for i := 0; i < len(s); i++ {
		hash ^= uint32(s[i])
		hash *= prime32
	}
	return hash
}

var (
	// ErrCacheNotEnabled is returned when cache operations are attempted on a disabled cache
	ErrCacheNotEnabled = errors.New("cache is not enabled")
	// ErrInvalidConfig is returned when cache configuration is invalid
	ErrInvalidConfig = errors.New("invalid cache configuration")
)

// ShardedCache is a thread-safe DNS response cache with multiple shards to reduce lock contention.
// Each shard operates independently with its own mutex, allowing concurrent access to different shards.
// Fields ordered for optimal memory alignment
type ShardedCache struct {
	shards      []*CacheShard   // Slice of cache shards (24 bytes)
	logger      *logging.Logger // Logger instance (8 bytes)
	stopCleanup chan struct{}   // Channel to stop cleanup (8 bytes)
	cleanupDone chan struct{}   // Channel signaling cleanup done (8 bytes)
	shardCount  int             // Number of shards (8 bytes)
}

// CacheShard represents a single shard of the cache with its own lock and entries.
// Stats are tracked using atomic operations to avoid lock contention on hot paths.
// Fields ordered for optimal memory alignment (reduces padding from 56 to 32 bytes)
type CacheShard struct {
	mu          sync.RWMutex           // Lock for entries map (largest field first)
	entries     map[string]*cacheEntry // Cache entries map
	cfg         *config.CacheConfig    // Cache configuration
	logger      *logging.Logger        // Logger instance
	metrics     *telemetry.Metrics     // Metrics recorder
	statsHits   atomic.Uint64          // Atomic counter for cache hits
	statsMisses atomic.Uint64          // Atomic counter for cache misses
	statsEvicts atomic.Uint64          // Atomic counter for evictions
	statsSets   atomic.Uint64          // Atomic counter for sets
	maxEntries  int                    // Maximum entries per shard
}

// NewSharded creates a new sharded DNS cache with the specified number of shards.
// shardCount should be a power of 2 for optimal performance (e.g., 16, 32, 64, 128).
func NewSharded(cfg *config.CacheConfig, logger *logging.Logger, metrics *telemetry.Metrics, shardCount int) (*ShardedCache, error) {
	if cfg == nil {
		return nil, ErrCacheNotEnabled
	}
	if logger == nil {
		logger = &logging.Logger{} // Create minimal logger
	}
	if shardCount <= 0 {
		shardCount = 64 // Default to 64 shards
	}

	// Validate config
	if cfg.MaxEntries <= 0 {
		return nil, ErrInvalidConfig
	}

	// Calculate entries per shard
	entriesPerShard := cfg.MaxEntries / shardCount
	if entriesPerShard < 10 {
		entriesPerShard = 10 // Minimum entries per shard
	}

	sc := &ShardedCache{
		shards:      make([]*CacheShard, shardCount),
		shardCount:  shardCount,
		logger:      logger,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Initialize each shard
	for i := 0; i < shardCount; i++ {
		sc.shards[i] = &CacheShard{
			cfg:        cfg,
			logger:     logger,
			metrics:    metrics,
			entries:    make(map[string]*cacheEntry, entriesPerShard),
			maxEntries: entriesPerShard,
		}
	}

	// Start background cleanup goroutine
	go sc.cleanupLoop()

	logger.Info("Sharded DNS cache initialized",
		"shards", shardCount,
		"entries_per_shard", entriesPerShard,
		"total_capacity", cfg.MaxEntries,
		"min_ttl", cfg.MinTTL,
		"max_ttl", cfg.MaxTTL)

	return sc, nil
}

// getShard returns the shard for the given key using FNV-1a hash.
// FNV-1a is chosen for its speed and good distribution properties.
// Uses inline hash function to avoid allocations from hash.Hash interface.
func (sc *ShardedCache) getShard(key string) *CacheShard {
	shardIdx := fnv1aHashString(key) % uint32(sc.shardCount)
	return sc.shards[shardIdx]
}

// Get retrieves a cached DNS response for the given request.
// Returns nil if not found or expired.
func (sc *ShardedCache) Get(ctx context.Context, r *dns.Msg) *dns.Msg {
	resp, _ := sc.GetWithTrace(ctx, r)
	return resp
}

// GetWithTrace returns the cached response and any associated block trace metadata.
func (sc *ShardedCache) GetWithTrace(ctx context.Context, r *dns.Msg) (*dns.Msg, []storage.BlockTraceEntry) {
	if len(r.Question) == 0 {
		return nil, nil
	}

	question := r.Question[0]
	key := makeKey(question.Name, question.Qtype)

	// Get the appropriate shard
	shard := sc.getShard(key)

	shard.mu.RLock()
	entry, found := shard.entries[key]
	shard.mu.RUnlock()

	if !found {
		sc.recordMiss(shard)
		return nil, nil
	}

	// Check if expired
	now := time.Now()
	if now.After(entry.expiresAt) {
		sc.recordMiss(shard)
		// Remove expired entry (upgrade to write lock)
		shard.mu.Lock()
		delete(shard.entries, key)

		// Record cache size decrease
		if shard.metrics != nil {
			shard.metrics.CacheSize.Add(ctx, -1)
		}

		shard.mu.Unlock()
		return nil, nil
	}

	// Update last access time (for LRU) using atomic operation - no lock needed
	atomic.StoreInt64(&entry.lastAccessNano, now.UnixNano())

	sc.recordHit(shard)

	// Return a copy to prevent mutations
	return entry.msg.Copy(), cloneBlockTrace(entry.blockTrace)
}

// Set stores a DNS response in the cache with appropriate TTL.
func (sc *ShardedCache) Set(ctx context.Context, r *dns.Msg, resp *dns.Msg) {
	if len(r.Question) == 0 {
		return
	}

	question := r.Question[0]
	key := makeKey(question.Name, question.Qtype)

	// Determine TTL from response
	ttl := determineTTL(sc.shards[0].cfg, resp)
	if ttl <= 0 {
		// Don't cache responses with zero or negative TTL
		return
	}

	now := time.Now()
	entry := &cacheEntry{
		msg:            resp.Copy(), // Deep copy to prevent mutations
		expiresAt:      now.Add(ttl),
		lastAccessNano: now.UnixNano(),
		size:           resp.Len(),
	}

	// Get the appropriate shard
	shard := sc.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Check if we need to evict entries (LRU)
	if len(shard.entries) >= shard.maxEntries {
		sc.evictLRU(shard)
	}

	// Check if this is a new entry or replacement
	_, exists := shard.entries[key]

	shard.entries[key] = entry

	// Use atomic increment for sets counter (lock-free)
	shard.statsSets.Add(1)

	// Record cache size change (only increment if new entry)
	if shard.metrics != nil && !exists {
		shard.metrics.CacheSize.Add(ctx, 1)
	}
}

// SetBlocked stores a blocked domain response in the cache with BlockedTTL.
func (sc *ShardedCache) SetBlocked(ctx context.Context, r *dns.Msg, resp *dns.Msg, trace []storage.BlockTraceEntry) {
	if len(r.Question) == 0 {
		return
	}

	question := r.Question[0]
	key := makeKey(question.Name, question.Qtype)

	// Use configured blocked TTL
	ttl := sc.shards[0].cfg.BlockedTTL
	if ttl <= 0 {
		// Don't cache if BlockedTTL is disabled
		return
	}

	now := time.Now()
	entry := &cacheEntry{
		msg:            resp.Copy(),
		expiresAt:      now.Add(ttl),
		lastAccessNano: now.UnixNano(),
		size:           resp.Len(),
		blockTrace:     cloneBlockTrace(trace),
	}

	// Get the appropriate shard
	shard := sc.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Check if we need to evict entries (LRU)
	if len(shard.entries) >= shard.maxEntries {
		sc.evictLRU(shard)
	}

	// Check if this is a new entry or replacement
	_, exists := shard.entries[key]

	shard.entries[key] = entry

	// Use atomic increment for sets counter (lock-free)
	shard.statsSets.Add(1)

	// Record cache size change (only increment if new entry)
	if shard.metrics != nil && !exists {
		shard.metrics.CacheSize.Add(ctx, 1)
	}
}

// evictLRU removes the least recently used entry from the given shard.
// Must be called with write lock held.
func (sc *ShardedCache) evictLRU(shard *CacheShard) {
	var oldestKey string
	var oldestNano int64 = 0

	// Find the entry with the oldest last access time
	for key, entry := range shard.entries {
		lastAccess := atomic.LoadInt64(&entry.lastAccessNano)
		if oldestKey == "" || lastAccess < oldestNano {
			oldestKey = key
			oldestNano = lastAccess
		}
	}

	if oldestKey != "" {
		delete(shard.entries, oldestKey)

		// Use atomic increment for evictions counter (lock-free)
		shard.statsEvicts.Add(1)

		// Record cache size decrease
		if shard.metrics != nil {
			shard.metrics.CacheSize.Add(context.Background(), -1)
		}
	}
}

// Stats returns aggregated cache statistics across all shards using atomic loads.
func (sc *ShardedCache) Stats() Stats {
	var aggregated Stats

	for _, shard := range sc.shards {
		// Read atomic counters without locking (lock-free)
		aggregated.Hits += shard.statsHits.Load()
		aggregated.Misses += shard.statsMisses.Load()
		aggregated.Evictions += shard.statsEvicts.Load()
		aggregated.Sets += shard.statsSets.Load()

		// Entry count still needs read lock (map access)
		shard.mu.RLock()
		aggregated.Entries += len(shard.entries)
		shard.mu.RUnlock()
	}

	total := aggregated.Hits + aggregated.Misses
	if total > 0 {
		aggregated.HitRate = float64(aggregated.Hits) / float64(total)
	}

	return aggregated
}

// Clear removes all entries from all shards.
func (sc *ShardedCache) Clear() {
	for _, shard := range sc.shards {
		shard.mu.Lock()
		oldSize := len(shard.entries)
		shard.entries = make(map[string]*cacheEntry, shard.maxEntries)

		// Record cache size decrease
		if shard.metrics != nil && oldSize > 0 {
			shard.metrics.CacheSize.Add(context.Background(), int64(-oldSize))
		}
		shard.mu.Unlock()
	}

	sc.logger.Info("Sharded cache cleared")
}

// Close stops the cache and cleanup goroutine.
func (sc *ShardedCache) Close() error {
	close(sc.stopCleanup)
	<-sc.cleanupDone

	stats := sc.Stats()
	sc.logger.Info("Sharded cache closed",
		"shards", sc.shardCount,
		"final_hits", stats.Hits,
		"final_misses", stats.Misses,
		"final_entries", stats.Entries,
		"hit_rate", stats.HitRate)

	return nil
}

// cleanupLoop runs in the background to remove expired entries from all shards.
func (sc *ShardedCache) cleanupLoop() {
	defer close(sc.cleanupDone)

	// Run cleanup every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sc.cleanup()
		case <-sc.stopCleanup:
			return
		}
	}
}

// cleanup removes all expired entries from all shards in parallel.
// By processing shards concurrently, cleanup time is reduced by up to shardCount times.
func (sc *ShardedCache) cleanup() {
	now := time.Now()
	var totalRemoved atomic.Uint64

	// Process all shards in parallel for maximum throughput
	var wg sync.WaitGroup
	wg.Add(sc.shardCount)

	for i := range sc.shards {
		go func(shard *CacheShard) {
			defer wg.Done()

			shard.mu.Lock()
			removed := 0
			for key, entry := range shard.entries {
				if now.After(entry.expiresAt) {
					delete(shard.entries, key)
					removed++
				}
			}
			shard.mu.Unlock()

			if removed > 0 {
				// Use atomic increment for evictions counter (lock-free)
				shard.statsEvicts.Add(uint64(removed))
				totalRemoved.Add(uint64(removed))
			}
		}(sc.shards[i])
	}

	// Wait for all shards to complete
	wg.Wait()

	removed := totalRemoved.Load()
	if removed > 0 {
		totalEntries := 0
		for _, shard := range sc.shards {
			shard.mu.RLock()
			totalEntries += len(shard.entries)
			shard.mu.RUnlock()
		}
		sc.logger.Debug("Cleaned up expired cache entries",
			"removed", removed,
			"remaining", totalEntries)
	}
}

// recordHit atomically increments the hit counter for a shard using lock-free operations.
func (sc *ShardedCache) recordHit(shard *CacheShard) {
	shard.statsHits.Add(1)

	// Record to Prometheus metrics
	if shard.metrics != nil {
		shard.metrics.DNSCacheHits.Add(context.Background(), 1)
	}
}

// recordMiss atomically increments the miss counter for a shard using lock-free operations.
func (sc *ShardedCache) recordMiss(shard *CacheShard) {
	shard.statsMisses.Add(1)

	// Record to Prometheus metrics
	if shard.metrics != nil {
		shard.metrics.DNSCacheMisses.Add(context.Background(), 1)
	}
}

// Helper functions (extracted from cache.go for reuse)

// makeKey creates a cache key from domain and query type.
func makeKey(domain string, qtype uint16) string {
	// Format: domain:qtype (using numeric type for consistency)
	// Example: "example.com.:1" (A record)
	// Note: We convert qtype to string to avoid allocations from fmt.Sprintf
	// Simple conversion for uint16 range (0-65535)
	var buf [5]byte
	i := len(buf)
	q := qtype
	for {
		i--
		buf[i] = byte('0' + q%10)
		q /= 10
		if q == 0 {
			break
		}
	}
	return domain + ":" + string(buf[i:])
}

// determineTTL extracts TTL from DNS response and applies min/max limits.
func determineTTL(cfg *config.CacheConfig, resp *dns.Msg) time.Duration {
	// For negative responses (NXDOMAIN, NODATA), use negative TTL
	if resp.Rcode == dns.RcodeNameError || len(resp.Answer) == 0 {
		return cfg.NegativeTTL
	}

	// Find minimum TTL from answer section
	var minTTL uint32 = 0
	for _, rr := range resp.Answer {
		ttl := rr.Header().Ttl
		if minTTL == 0 || ttl < minTTL {
			minTTL = ttl
		}
	}

	// If no TTL found, use negative TTL
	if minTTL == 0 {
		return cfg.NegativeTTL
	}

	// Convert to duration and apply limits
	ttl := time.Duration(minTTL) * time.Second

	if ttl < cfg.MinTTL {
		ttl = cfg.MinTTL
	}
	if ttl > cfg.MaxTTL {
		ttl = cfg.MaxTTL
	}

	return ttl
}
