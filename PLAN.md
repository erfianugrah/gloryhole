# High Priority Performance Optimizations

These are easy wins with high impact on the hot path.

## Status: COMPLETED

## Changes Made

1. **Regex caching** - `pkg/policy/engine.go`: Added `sync.Map` cache for compiled regexes
2. **Inline FNV hash** - `pkg/cache/sharded_cache.go`: Replaced `hash/fnv` with allocation-free inline FNV-1a
3. **Atomic cache stats** - `pkg/cache/cache.go`: Changed stats from mutex-protected to atomic operations
4. **Optimized makeKey** - `pkg/cache/cache.go`: Replaced `fmt.Sprintf` with manual integer conversion
5. **Legacy log worker pool** - `pkg/dns/handler.go`: Replaced per-query goroutine spawn with channel+worker pool

## Issues to Fix

### 1. Regex Recompilation in DomainRegex (CRITICAL)
**File:** `pkg/policy/engine.go:398-404`

**Problem:** `regexp.MatchString` compiles the regex on EVERY call. This is extremely expensive.

**Current Code:**
```go
func DomainRegex(domain, pattern string) (bool, error) {
    matched, err := regexp.MatchString(pattern, strings.ToLower(domain))
    if err != nil {
        return false, fmt.Errorf("invalid regex pattern: %w", err)
    }
    return matched, nil
}
```

**Solution:** Cache compiled regexes using sync.Map:
```go
var regexCache sync.Map

func DomainRegex(domain, pattern string) (bool, error) {
    cached, ok := regexCache.Load(pattern)
    if !ok {
        re, err := regexp.Compile(pattern)
        if err != nil {
            return false, fmt.Errorf("invalid regex pattern: %w", err)
        }
        regexCache.Store(pattern, re)
        cached = re
    }
    
    re := cached.(*regexp.Regexp)
    return re.MatchString(strings.ToLower(domain)), nil
}
```

**Impact:** HIGH - regex compilation is very expensive

---

### 2. FNV Hash Allocation in Sharded Cache
**File:** `pkg/cache/sharded_cache.go:111-116`

**Problem:** `fnv.New32a()` allocates a new hash object AND `[]byte(key)` allocates on every cache operation.

**Current Code:**
```go
func (sc *ShardedCache) getShard(key string) *CacheShard {
    h := fnv.New32a()
    _, _ = h.Write([]byte(key))
    shardIdx := h.Sum32() % uint32(sc.shardCount)
    return sc.shards[shardIdx]
}
```

**Solution:** Inline FNV-1a hash without allocations:
```go
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

func (sc *ShardedCache) getShard(key string) *CacheShard {
    shardIdx := fnv1aHashString(key) % uint32(sc.shardCount)
    return sc.shards[shardIdx]
}
```

**Impact:** HIGH - cache shard selection on every cache operation

---

### 3. Cache Stats Under Mutex (Non-Sharded Cache)
**File:** `pkg/cache/cache.go:443-458`

**Problem:** Non-sharded cache uses mutex for stats updates. Sharded cache already uses atomics.

**Current Code:**
```go
func (c *Cache) recordHit() {
    c.mu.Lock()
    c.stats.hits++
    c.mu.Unlock()
    // ...
}

func (c *Cache) recordMiss() {
    c.mu.Lock()
    c.stats.misses++
    c.mu.Unlock()
    // ...
}
```

**Solution:** Use atomic operations (matching sharded cache pattern):
```go
type cacheStats struct {
    hits      atomic.Uint64
    misses    atomic.Uint64
    entries   atomic.Int32
    evictions atomic.Uint64
    sets      atomic.Uint64
}

func (c *Cache) recordHit() {
    c.stats.hits.Add(1)
    if c.metrics != nil {
        c.metrics.DNSCacheHits.Add(context.Background(), 1)
    }
}
```

**Impact:** HIGH - contention on every cache hit/miss

---

### 4. Cache Key fmt.Sprintf (Non-Sharded Cache)
**File:** `pkg/cache/cache.go:284-288`

**Problem:** Non-sharded cache uses `fmt.Sprintf` for key generation. Sharded cache has the optimized version.

**Current Code:**
```go
func (c *Cache) makeKey(domain string, qtype uint16) string {
    return fmt.Sprintf("%s:%d", domain, qtype)
}
```

**Solution:** Use the same optimized version from sharded_cache.go:
```go
func (c *Cache) makeKey(domain string, qtype uint16) string {
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
```

**Impact:** HIGH - cache key generation on every lookup

---

### 5. Legacy Goroutine Spawn Per Query
**File:** `pkg/dns/handler.go:272-284`

**Problem:** Legacy logging path spawns a new goroutine for every single DNS query.

**Current Code:**
```go
// Legacy path: spawn goroutine (backwards compatibility)
go func() {
    logCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    if err := h.Storage.LogQuery(logCtx, queryLog); err != nil && h.Logger != nil {
        // ...
    }
}()
```

**Solution:** Remove legacy path or make it use a worker pool. Since QueryLogger exists, we can make the legacy storage auto-wrap:
```go
// In asyncLogQuery, if QueryLogger is nil but Storage exists, 
// create a simple channel-based worker instead of spawning goroutines
```

Or simpler: deprecate the legacy path and require QueryLogger to be set.

**Impact:** HIGH for legacy path users - goroutine overhead per query

---

## Testing Strategy

1. Run existing unit tests: `go test ./pkg/cache/... ./pkg/policy/... ./pkg/dns/...`
2. Run integration tests: `go test ./test/...`
3. Build: `go build ./cmd/glory-hole`
4. Lint: `golangci-lint run`

## Verification

After implementing, verify with a simple benchmark:
```go
// In engine_test.go
func BenchmarkDomainRegex(b *testing.B) {
    for i := 0; i < b.N; i++ {
        DomainRegex("example.com", "^.*\\.example\\.com$")
    }
}

// In sharded_cache_test.go
func BenchmarkGetShard(b *testing.B) {
    sc, _ := NewSharded(cfg, logger, nil, 64)
    for i := 0; i < b.N; i++ {
        sc.getShard("example.com.:1")
    }
}
```
