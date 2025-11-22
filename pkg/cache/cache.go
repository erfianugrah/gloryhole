package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"github.com/miekg/dns"
)

// Cache is a thread-safe DNS response cache with LRU eviction and TTL support
type Cache struct {
	cfg         *config.CacheConfig
	logger      *logging.Logger
	entries     map[string]*cacheEntry
	stopCleanup chan struct{}
	cleanupDone chan struct{}
	stats       cacheStats
	maxEntries  int
	mu          sync.RWMutex
}

// cacheEntry holds a cached DNS response with metadata
type cacheEntry struct {
	// The cached DNS response (deep copy to avoid mutations)
	msg *dns.Msg

	// When this entry expires (based on DNS TTL)
	expiresAt time.Time

	// When this entry was last accessed (for LRU eviction)
	lastAccess time.Time

	// Size in bytes (for memory tracking)
	size int
}

// cacheStats tracks cache performance metrics
type cacheStats struct {
	hits      uint64 // Cache hits
	misses    uint64 // Cache misses
	entries   int    // Current number of entries
	evictions uint64 // Number of evictions (LRU or TTL)
	sets      uint64 // Number of cache sets
}

// Stats returns a copy of the current cache statistics
type Stats struct {
	Hits      uint64
	Misses    uint64
	Entries   int
	Evictions uint64
	Sets      uint64
	HitRate   float64 // hits / (hits + misses)
}

// New creates a new DNS cache with the given configuration
func New(cfg *config.CacheConfig, logger *logging.Logger) (*Cache, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cache config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	if cfg.MaxEntries <= 0 {
		return nil, fmt.Errorf("max_entries must be positive, got %d", cfg.MaxEntries)
	}

	c := &Cache{
		cfg:         cfg,
		logger:      logger,
		entries:     make(map[string]*cacheEntry, cfg.MaxEntries),
		maxEntries:  cfg.MaxEntries,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go c.cleanupLoop()

	logger.Info("DNS cache initialized",
		"max_entries", cfg.MaxEntries,
		"min_ttl", cfg.MinTTL,
		"max_ttl", cfg.MaxTTL,
		"negative_ttl", cfg.NegativeTTL)

	return c, nil
}

// Get retrieves a cached DNS response for the given request
// Returns nil if not found or expired
func (c *Cache) Get(ctx context.Context, r *dns.Msg) *dns.Msg {
	if !c.cfg.Enabled {
		return nil
	}

	if len(r.Question) == 0 {
		return nil
	}

	key := c.makeKey(r.Question[0].Name, r.Question[0].Qtype)

	c.mu.RLock()
	entry, found := c.entries[key]
	c.mu.RUnlock()

	if !found {
		c.recordMiss()
		return nil
	}

	// Check if expired
	now := time.Now()
	if now.After(entry.expiresAt) {
		c.recordMiss()
		// Remove expired entry (upgrade to write lock)
		c.mu.Lock()
		delete(c.entries, key)
		c.stats.entries--
		c.mu.Unlock()
		return nil
	}

	// Update last access time (for LRU)
	c.mu.Lock()
	entry.lastAccess = now
	c.mu.Unlock()

	c.recordHit()

	// Return a copy to prevent mutations
	return entry.msg.Copy()
}

// Set stores a DNS response in the cache with appropriate TTL
func (c *Cache) Set(ctx context.Context, r *dns.Msg, resp *dns.Msg) {
	if !c.cfg.Enabled {
		return
	}

	if len(r.Question) == 0 {
		return
	}

	question := r.Question[0]
	key := c.makeKey(question.Name, question.Qtype)

	// Determine TTL from response
	ttl := c.determineTTL(resp)
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

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries (LRU)
	if len(c.entries) >= c.maxEntries {
		c.evictLRU()
	}

	c.entries[key] = entry
	c.stats.entries = len(c.entries)
	c.stats.sets++

	c.mu.Unlock()
	c.logger.Debug("Cached DNS response",
		"domain", question.Name,
		"qtype", dns.TypeToString[question.Qtype],
		"ttl", ttl,
		"size", entry.size)
	c.mu.Lock()
}

// makeKey creates a cache key from domain and query type
func (c *Cache) makeKey(domain string, qtype uint16) string {
	// Format: domain:qtype
	// Example: "example.com.:A"
	return fmt.Sprintf("%s:%d", domain, qtype)
}

// determineTTL extracts TTL from DNS response and applies min/max limits
func (c *Cache) determineTTL(resp *dns.Msg) time.Duration {
	// For negative responses (NXDOMAIN, NODATA), use negative TTL
	if resp.Rcode == dns.RcodeNameError || len(resp.Answer) == 0 {
		return c.cfg.NegativeTTL
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
		return c.cfg.NegativeTTL
	}

	// Convert to duration and apply limits
	ttl := time.Duration(minTTL) * time.Second

	if ttl < c.cfg.MinTTL {
		ttl = c.cfg.MinTTL
	}
	if ttl > c.cfg.MaxTTL {
		ttl = c.cfg.MaxTTL
	}

	return ttl
}

// evictLRU removes the least recently used entry
// Must be called with write lock held
func (c *Cache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time

	// Find the entry with the oldest last access time
	for key, entry := range c.entries {
		if oldestKey == "" || entry.lastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastAccess
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
		c.stats.evictions++
		c.logger.Debug("Evicted LRU cache entry", "key", oldestKey)
	}
}

// cleanupLoop runs in the background to remove expired entries
func (c *Cache) cleanupLoop() {
	defer close(c.cleanupDone)

	// Run cleanup every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanup removes all expired entries
func (c *Cache) cleanup() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
			removed++
		}
	}

	if removed > 0 {
		c.stats.evictions += uint64(removed)
		c.stats.entries = len(c.entries)
		c.logger.Debug("Cleaned up expired cache entries", "removed", removed, "remaining", c.stats.entries)
	}
}

// Stats returns current cache statistics
func (c *Cache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.stats.hits + c.stats.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.stats.hits) / float64(total)
	}

	return Stats{
		Hits:      c.stats.hits,
		Misses:    c.stats.misses,
		Entries:   c.stats.entries,
		Evictions: c.stats.evictions,
		Sets:      c.stats.sets,
		HitRate:   hitRate,
	}
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry, c.maxEntries)
	c.stats.entries = 0
	c.logger.Info("Cache cleared")
}

// Close stops the cache and cleanup goroutine
func (c *Cache) Close() error {
	close(c.stopCleanup)
	<-c.cleanupDone

	c.logger.Info("Cache closed",
		"final_hits", c.stats.hits,
		"final_misses", c.stats.misses,
		"final_entries", c.stats.entries)

	return nil
}

// recordHit atomically increments the hit counter
func (c *Cache) recordHit() {
	c.mu.Lock()
	c.stats.hits++
	c.mu.Unlock()
}

// recordMiss atomically increments the miss counter
func (c *Cache) recordMiss() {
	c.mu.Lock()
	c.stats.misses++
	c.mu.Unlock()
}
