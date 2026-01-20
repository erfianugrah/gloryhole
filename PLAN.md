# Low Priority Performance Optimizations

Higher effort changes with good long-term benefits.

## Status: COMPLETED (Partial)

## Changes Made

1. **Probabilistic LRU eviction** - `pkg/cache/cache.go`, `pkg/cache/sharded_cache.go`: Changed from O(n) full scan to O(1) sampling approach (like Redis). Samples 5 random entries and evicts the oldest.

2. **Domain stats worker channel** - `pkg/storage/sqlite.go`: Replaced per-batch goroutine spawning with a dedicated worker and buffered channel.

3. **Batched domain stats updates** - `pkg/storage/sqlite.go`: Groups queries by domain before SQL updates to reduce number of SQL statements. Uses single transaction for all updates.

## Not Implemented (Future Work)

- Suffix trie for wildcard patterns (high complexity)
- CIDR radix trie (requires new dependency)
- Proper doubly-linked list LRU (higher complexity than sampling)

## Issues to Fix

### 1. LRU Eviction via Full Map Scan - O(n)
**File:** `pkg/cache/cache.go:326-349` and `pkg/cache/sharded_cache.go:279-302`

**Problem:** LRU eviction scans ALL entries to find the oldest. With 100K+ entries, this is slow.

**Current Code:**
```go
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
    // ...
}
```

**Solution Options:**

**Option A: Use container/list for doubly-linked list (proper LRU)**
```go
import "container/list"

type LRUCache struct {
    entries   map[string]*list.Element
    evictList *list.List
    maxSize   int
}

type lruEntry struct {
    key   string
    value *cacheEntry
}

func (c *LRUCache) Get(key string) *cacheEntry {
    if elem, ok := c.entries[key]; ok {
        c.evictList.MoveToFront(elem)  // O(1)
        return elem.Value.(*lruEntry).value
    }
    return nil
}

func (c *LRUCache) evict() {
    elem := c.evictList.Back()  // O(1)
    if elem != nil {
        c.evictList.Remove(elem)
        delete(c.entries, elem.Value.(*lruEntry).key)
    }
}
```

**Option B: Use a third-party LRU like github.com/hashicorp/golang-lru**

**Option C: Probabilistic eviction (random sampling)**
```go
func (c *Cache) evictLRU() {
    // Sample 5 random entries, evict the oldest
    const sampleSize = 5
    var candidates [sampleSize]struct {
        key  string
        time time.Time
    }
    
    i := 0
    for key, entry := range c.entries {
        if i < sampleSize {
            candidates[i] = struct{key string; time time.Time}{key, entry.lastAccess}
            i++
        } else {
            break
        }
    }
    
    // Find oldest in sample
    oldest := 0
    for j := 1; j < i; j++ {
        if candidates[j].time.Before(candidates[oldest].time) {
            oldest = j
        }
    }
    
    delete(c.entries, candidates[oldest].key)
}
```

**Impact:** HIGH for large caches - eviction goes from O(n) to O(1)
**Effort:** HIGH - significant refactoring

---

### 2. Wildcard/Regex Patterns Linear Scan - O(n)
**File:** `pkg/pattern/pattern.go` (if applicable)

**Problem:** Wildcard patterns are scanned linearly. For blocklists with many wildcard rules, this is slow.

**Current Code:**
```go
// Try wildcards (fast)
for _, pattern := range m.wildcard {
    if pattern.Match(domain) {
        return pattern, true
    }
}
```

**Solution:** For suffix-based wildcards (`*.example.com`), use a suffix trie:
```go
type SuffixTrie struct {
    root *trieNode
}

type trieNode struct {
    children map[string]*trieNode
    pattern  *Pattern
}

// Lookup example.foo.bar.com by splitting and walking trie:
// com -> bar -> foo -> example
func (t *SuffixTrie) Match(domain string) (*Pattern, bool) {
    parts := strings.Split(domain, ".")
    node := t.root
    
    // Walk from TLD to subdomain
    for i := len(parts) - 1; i >= 0; i-- {
        if node.pattern != nil {
            return node.pattern, true  // Wildcard match
        }
        child, ok := node.children[parts[i]]
        if !ok {
            return nil, false
        }
        node = child
    }
    
    return node.pattern, node.pattern != nil
}
```

**Impact:** MEDIUM - depends on number of wildcard patterns
**Effort:** HIGH - new data structure

---

### 3. CIDR Matching Linear Scan
**File:** `pkg/forwarder/matcher.go` (if applicable)

**Problem:** CIDR ranges scanned linearly.

**Solution:** Use a radix trie like `github.com/yl2chen/cidranger`:
```go
import "github.com/yl2chen/cidranger"

type CIDRMatcher struct {
    ranger cidranger.Ranger
}

func (cm *CIDRMatcher) Matches(ipStr string) bool {
    ip := net.ParseIP(ipStr)
    contains, _ := cm.ranger.Contains(ip)
    return contains
}
```

**Impact:** LOW-MEDIUM - only matters with many CIDR rules
**Effort:** LOW - uses existing library

---

### 4. Domain Stats Update Goroutine Per Batch
**File:** `pkg/storage/sqlite.go:320-324`

**Problem:** Spawns goroutine per flush batch for domain stats updates.

**Current Code:**
```go
queriesCopy := make([]*QueryLog, len(queries))
copy(queriesCopy, queries)
go s.updateDomainStats(queriesCopy)
```

**Solution:** Use dedicated worker goroutine with channel:
```go
type SQLiteStorage struct {
    // ...
    domainStatsCh chan []*QueryLog
}

func NewSQLiteStorage(...) *SQLiteStorage {
    s := &SQLiteStorage{
        domainStatsCh: make(chan []*QueryLog, 100),
    }
    go s.domainStatsWorker()
    return s
}

func (s *SQLiteStorage) domainStatsWorker() {
    for batch := range s.domainStatsCh {
        s.updateDomainStats(batch)
    }
}
```

**Impact:** LOW - goroutine per batch, not per query
**Effort:** LOW

---

### 5. Batch Domain Stats SQL Updates
**File:** `pkg/storage/sqlite.go:343-361`

**Problem:** Each domain in batch triggers separate SQL statement.

**Solution:** Group and batch:
```go
func (s *SQLiteStorage) updateDomainStats(queries []*QueryLog) {
    // Group by domain
    domainUpdates := make(map[string]*domainStatUpdate)
    for _, q := range queries {
        if u, ok := domainUpdates[q.Domain]; ok {
            u.count++
        } else {
            domainUpdates[q.Domain] = &domainStatUpdate{count: 1, ...}
        }
    }
    
    // Single transaction
    tx, _ := s.db.Begin()
    stmt, _ := tx.Prepare(`INSERT ... ON CONFLICT ...`)
    for domain, update := range domainUpdates {
        stmt.Exec(domain, update.count, ...)
    }
    tx.Commit()
}
```

**Impact:** MEDIUM - reduces DB operations
**Effort:** MEDIUM

---

## Testing Strategy

1. Run all tests: `go test ./...`
2. Build: `go build ./cmd/glory-hole`
3. Lint: `golangci-lint run`
4. Benchmark with large cache/blocklist sizes

## Implementation Order

1. CIDR radix trie (if many rules) - easy, uses library
2. Domain stats batching - moderate complexity
3. LRU data structure - most complex, most benefit
4. Suffix trie for wildcards - complex, depends on usage patterns
