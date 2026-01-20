# Medium Priority Performance Optimizations

These optimizations reduce allocations and lock contention on hot paths.

## Status: COMPLETED

## Changes Made

1. **Atomic lastAccess for LRU** - `pkg/cache/cache.go`, `pkg/cache/sharded_cache.go`: Replaced time.Time with int64 (UnixNano) for atomic operations, eliminating write lock on cache hits
2. **sync.Pool for serveDNSOutcome** - `pkg/dns/handler_state.go`: Added object pool to avoid per-query allocation
3. **sync.Pool for blockTraceRecorder** - `pkg/dns/trace.go`: Added object pool with pre-allocated slice capacity
4. **Updated handler.go** - Uses pooled objects with proper defer cleanup

## Issues to Fix

### 1. Cache Hit Write Lock for LRU Update
**File:** `pkg/cache/sharded_cache.go:163-170`

**Problem:** Every cache HIT requires a write lock just to update `lastAccess` timestamp.

**Current Code:**
```go
// Update last access time (for LRU)
shard.mu.Lock()
entry.lastAccess = now
shard.mu.Unlock()
```

**Solution:** Use atomic int64 for lastAccess:
```go
type cacheEntry struct {
    // ... other fields
    lastAccessNano int64  // Atomic, store as UnixNano
}

// On hit (lock-free):
atomic.StoreInt64(&entry.lastAccessNano, now.UnixNano())

// In evictLRU, read with:
lastAccess := time.Unix(0, atomic.LoadInt64(&entry.lastAccessNano))
```

**Impact:** HIGH - every cache hit currently contends for write lock

---

### 2. Policy Engine RWMutex on Every Query
**File:** `pkg/policy/engine.go:185-188`

**Problem:** Every policy evaluation takes a read lock, even though rules rarely change.

**Current Code:**
```go
func (e *Engine) Evaluate(ctx Context) (bool, *Rule) {
    e.mu.RLock()
    defer e.mu.RUnlock()

    for _, rule := range e.rules {
        // ...
    }
}
```

**Solution:** Use atomic pointer swap for rules (like blocklist manager):
```go
type Engine struct {
    rules  atomic.Pointer[[]*Rule]  // Atomic pointer to rules slice
    logger *logging.Logger
}

func (e *Engine) Evaluate(ctx Context) (bool, *Rule) {
    rulesPtr := e.rules.Load()
    if rulesPtr == nil {
        return false, nil
    }
    rules := *rulesPtr
    for _, rule := range rules {
        // ...
    }
}

func (e *Engine) SetRules(newRules []*Rule) {
    e.rules.Store(&newRules)
}
```

**Impact:** MEDIUM - removes lock contention on policy evaluation

---

### 3. Missing sync.Pool for serveDNSOutcome
**File:** `pkg/dns/handler.go:166`

**Problem:** `serveDNSOutcome` allocated on every DNS query.

**Current Code:**
```go
outcome := &serveDNSOutcome{}
```

**Solution:** Use sync.Pool:
```go
var outcomePool = sync.Pool{
    New: func() interface{} {
        return &serveDNSOutcome{}
    },
}

// In ServeDNS:
outcome := outcomePool.Get().(*serveDNSOutcome)
*outcome = serveDNSOutcome{} // Zero out
defer outcomePool.Put(outcome)
```

**Impact:** MEDIUM - reduces allocation per query

---

### 4. Missing sync.Pool for blockTraceRecorder
**File:** `pkg/dns/trace.go:18-19`

**Problem:** `blockTraceRecorder` allocated on every DNS query.

**Current Code:**
```go
func newBlockTraceRecorder(enabled bool) *blockTraceRecorder {
    return &blockTraceRecorder{enabled: enabled}
}
```

**Solution:** Use sync.Pool:
```go
var traceRecorderPool = sync.Pool{
    New: func() interface{} {
        return &blockTraceRecorder{
            entries: make([]storage.BlockTraceEntry, 0, 4),
        }
    },
}

func newBlockTraceRecorder(enabled bool) *blockTraceRecorder {
    r := traceRecorderPool.Get().(*blockTraceRecorder)
    r.enabled = enabled
    r.entries = r.entries[:0]
    return r
}

func (r *blockTraceRecorder) Release() {
    traceRecorderPool.Put(r)
}
```

Then in handler.go:
```go
trace := newBlockTraceRecorder(h.DecisionTrace)
defer trace.Release()
```

**Impact:** MEDIUM - reduces allocation per query

---

### 5. Pattern Allocation on Every Exact Match
**File:** `pkg/pattern/pattern.go:180-184` (if exists)

**Problem:** New `Pattern` struct allocated on every exact match return.

**Solution:** Store pre-created patterns in the exact map:
```go
type Matcher struct {
    exact    map[string]*Pattern  // Store pointers to pre-created patterns
    wildcard []*Pattern
    regex    []*Pattern
}
```

**Impact:** MEDIUM - allocation on every blocklist exact match

---

### 6. Repeated ToLower in Blocklist Manager
**File:** `pkg/blocklist/manager.go:298-305`

**Problem:** `strings.ToLower()` allocates a new string on every call.

**Current Code:**
```go
func (m *Manager) Match(domain string) MatchResult {
    normalized := strings.ToLower(domain)
    // ...
}
```

**Solution:** Check if lowercase is needed first:
```go
func (m *Manager) Match(domain string) MatchResult {
    // Fast path: check if already lowercase
    needsLower := false
    for i := 0; i < len(domain); i++ {
        if domain[i] >= 'A' && domain[i] <= 'Z' {
            needsLower = true
            break
        }
    }
    
    var normalized string
    if needsLower {
        normalized = strings.ToLower(domain)
    } else {
        normalized = domain
    }
    // ...
}
```

Better approach: normalize domain ONCE in handler.go before any lookups.

**Impact:** MEDIUM - allocation on every non-cached blocklist check

---

## Testing Strategy

1. Run unit tests: `go test ./pkg/...`
2. Run integration tests: `go test ./test/...`
3. Build: `go build ./cmd/glory-hole`
4. Lint: `golangci-lint run`

## Verification

Benchmark the improvements:
```go
func BenchmarkServeDNS(b *testing.B) {
    // Setup handler with all components
    // Run b.N queries and measure allocations
}
```

Use `-benchmem` flag to verify allocation reductions.
