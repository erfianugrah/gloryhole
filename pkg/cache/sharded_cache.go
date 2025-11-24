package cache

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/storage"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
)

var (
	// ErrCacheNotEnabled is returned when cache operations are attempted on a disabled cache
	ErrCacheNotEnabled = errors.New("cache is not enabled")
	// ErrInvalidConfig is returned when cache configuration is invalid
	ErrInvalidConfig = errors.New("invalid cache configuration")
)

// ShardedCache is a thread-safe DNS response cache with multiple shards to reduce lock contention.
// Each shard operates independently with its own mutex, allowing concurrent access to different shards.
type ShardedCache struct {
	shards      []*CacheShard
	shardCount  int
	logger      *logging.Logger
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// CacheShard represents a single shard of the cache with its own lock and entries.
type CacheShard struct {
	cfg         *config.CacheConfig
	logger      *logging.Logger
	metrics     *telemetry.Metrics
	entries     map[string]*cacheEntry
	maxEntries  int
	stats       cacheStats
	mu          sync.RWMutex
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
func (sc *ShardedCache) getShard(key string) *CacheShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	shardIdx := h.Sum32() % uint32(sc.shardCount)
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
		shard.stats.entries--

		// Record cache size decrease
		if shard.metrics != nil {
			shard.metrics.CacheSize.Add(ctx, -1)
		}

		shard.mu.Unlock()
		return nil, nil
	}

	// Update last access time (for LRU)
	shard.mu.Lock()
	entry.lastAccess = now
	shard.mu.Unlock()

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
		msg:        resp.Copy(), // Deep copy to prevent mutations
		expiresAt:  now.Add(ttl),
		lastAccess: now,
		size:       resp.Len(),
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
	shard.stats.entries = len(shard.entries)
	shard.stats.sets++

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
		msg:        resp.Copy(),
		expiresAt:  now.Add(ttl),
		lastAccess: now,
		size:       resp.Len(),
		blockTrace: cloneBlockTrace(trace),
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
	shard.stats.entries = len(shard.entries)
	shard.stats.sets++

	// Record cache size change (only increment if new entry)
	if shard.metrics != nil && !exists {
		shard.metrics.CacheSize.Add(ctx, 1)
	}
}

// evictLRU removes the least recently used entry from the given shard.
// Must be called with write lock held.
func (sc *ShardedCache) evictLRU(shard *CacheShard) {
	var oldestKey string
	var oldestTime time.Time

	// Find the entry with the oldest last access time
	for key, entry := range shard.entries {
		if oldestKey == "" || entry.lastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastAccess
		}
	}

	if oldestKey != "" {
		delete(shard.entries, oldestKey)
		shard.stats.evictions++

		// Record cache size decrease
		if shard.metrics != nil {
			shard.metrics.CacheSize.Add(context.Background(), -1)
		}
	}
}

// Stats returns aggregated cache statistics across all shards.
func (sc *ShardedCache) Stats() Stats {
	var aggregated Stats

	for _, shard := range sc.shards {
		shard.mu.RLock()
		aggregated.Hits += shard.stats.hits
		aggregated.Misses += shard.stats.misses
		aggregated.Entries += shard.stats.entries
		aggregated.Evictions += shard.stats.evictions
		aggregated.Sets += shard.stats.sets
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
		shard.stats.entries = 0

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

// cleanup removes all expired entries from all shards.
func (sc *ShardedCache) cleanup() {
	now := time.Now()
	totalRemoved := 0

	for _, shard := range sc.shards {
		shard.mu.Lock()
		removed := 0
		for key, entry := range shard.entries {
			if now.After(entry.expiresAt) {
				delete(shard.entries, key)
				removed++
			}
		}

		if removed > 0 {
			shard.stats.evictions += uint64(removed)
			shard.stats.entries = len(shard.entries)
			totalRemoved += removed
		}
		shard.mu.Unlock()
	}

	if totalRemoved > 0 {
		totalEntries := 0
		for _, shard := range sc.shards {
			shard.mu.RLock()
			totalEntries += shard.stats.entries
			shard.mu.RUnlock()
		}
		sc.logger.Debug("Cleaned up expired cache entries",
			"removed", totalRemoved,
			"remaining", totalEntries)
	}
}

// recordHit atomically increments the hit counter for a shard.
func (sc *ShardedCache) recordHit(shard *CacheShard) {
	shard.mu.Lock()
	shard.stats.hits++
	shard.mu.Unlock()

	// Record to Prometheus metrics
	if shard.metrics != nil {
		shard.metrics.DNSCacheHits.Add(context.Background(), 1)
	}
}

// recordMiss atomically increments the miss counter for a shard.
func (sc *ShardedCache) recordMiss(shard *CacheShard) {
	shard.mu.Lock()
	shard.stats.misses++
	shard.mu.Unlock()

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
