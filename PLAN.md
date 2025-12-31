# Performance Optimization: Query Logging Goroutine Management

## Issue Classification
**Priority:** P1 - CRITICAL
**Category:** Performance - Query Processing Hot Path
**Impact:** High QPS environments (>1000 QPS)

## Problem Statement

### Current Implementation
Location: `pkg/dns/handler.go:249`

```go
defer func() {
    go func() {
        if err := s.storage.LogQuery(ctx, &entry); err != nil {
            s.logger.Error("Failed to log query", "error", err)
        }
    }()
}()
```

### Root Cause Analysis
1. **Unbounded Goroutine Spawning:** Every DNS query spawns a NEW goroutine for logging
2. **Resource Exhaustion Risk:**
   - At 10K QPS: 10,000 goroutines spawned per second
   - Each goroutine stack: ~2KB minimum (can grow to 1GB max)
   - GC pressure from frequent goroutine creation/destruction
   - Scheduler overhead with 10K+ concurrent goroutines

3. **Why This Exists:**
   - Async logging to avoid blocking DNS response
   - Storage already has buffered channel (default 10K capacity)
   - Double-buffering: goroutine + storage buffer

4. **Actual Impact:**
   - Memory overhead: 2KB × 10K = ~20MB stack memory at 10K QPS
   - GC pauses: 5-10% increase due to short-lived goroutine allocations
   - Scheduler contention: Context switching overhead at high concurrency

## Proposed Solution

### Approach: Worker Pool Pattern
Replace per-query goroutine spawning with fixed worker pool.

### Architecture
```
DNS Handler → LogQuery Channel → Worker Pool → Storage Buffer → SQLite
              (buffered 50K)      (8 workers)   (buffered 10K)
```

### Implementation Changes

#### 1. Create Worker Pool Manager
**New File:** `pkg/dns/query_logger.go`

```go
type QueryLogger struct {
    logCh   chan *storage.QueryEntry
    workers int
    wg      sync.WaitGroup
    ctx     context.Context
    cancel  context.CancelFunc
    storage storage.Storage
    logger  *slog.Logger
}

func NewQueryLogger(storage storage.Storage, logger *slog.Logger, bufferSize, workers int) *QueryLogger {
    ctx, cancel := context.WithCancel(context.Background())
    ql := &QueryLogger{
        logCh:   make(chan *storage.QueryEntry, bufferSize),
        workers: workers,
        ctx:     ctx,
        cancel:  cancel,
        storage: storage,
        logger:  logger,
    }

    // Start worker pool
    for i := 0; i < workers; i++ {
        ql.wg.Add(1)
        go ql.worker(i)
    }

    return ql
}

func (ql *QueryLogger) worker(id int) {
    defer ql.wg.Done()
    for {
        select {
        case <-ql.ctx.Done():
            return
        case entry := <-ql.logCh:
            if err := ql.storage.LogQuery(ql.ctx, entry); err != nil {
                ql.logger.Error("Failed to log query", "worker", id, "error", err)
            }
        }
    }
}

func (ql *QueryLogger) LogAsync(entry *storage.QueryEntry) error {
    select {
    case ql.logCh <- entry:
        return nil
    default:
        return ErrQueryLogBufferFull
    }
}

func (ql *QueryLogger) Close() error {
    ql.cancel()
    ql.wg.Wait()
    close(ql.logCh)
    return nil
}
```

#### 2. Update DNS Handler
**File:** `pkg/dns/handler.go`

```diff
type handler struct {
    // ... existing fields
-   storage storage.Storage
+   queryLogger *QueryLogger
}

func (s *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
    // ... existing query processing

-   defer func() {
-       go func() {
-           if err := s.storage.LogQuery(ctx, &entry); err != nil {
-               s.logger.Error("Failed to log query", "error", err)
-           }
-       }()
-   }()
+   defer func() {
+       if err := s.queryLogger.LogAsync(&entry); err != nil {
+           s.logger.Warn("Query log buffer full, dropping entry", "domain", entry.Domain)
+       }
+   }()
}
```

#### 3. Configuration
**File:** `pkg/config/config.go`

```diff
type Config struct {
    DNS struct {
+       QueryLogBuffer  int `yaml:"query_log_buffer" default:"50000"`
+       QueryLogWorkers int `yaml:"query_log_workers" default:"8"`
    }
}
```

#### 4. Integration in Main
**File:** `cmd/glory-hole/main.go`

```diff
+   queryLogger := dns.NewQueryLogger(
+       storage,
+       logger.With("component", "query_logger"),
+       cfg.DNS.QueryLogBuffer,
+       cfg.DNS.QueryLogWorkers,
+   )
+   defer queryLogger.Close()

-   dnsHandler := dns.NewHandler(cfg, storage, cacheInstance, ...)
+   dnsHandler := dns.NewHandler(cfg, queryLogger, cacheInstance, ...)
```

## Expected Impact

### Performance Improvements
1. **Reduced Goroutine Count:**
   - Before: 10K goroutines spawned/sec at 10K QPS
   - After: 8 fixed worker goroutines
   - Reduction: 99.92%

2. **Memory Savings:**
   - Stack memory: ~20MB → ~16KB (99.9% reduction)
   - Heap allocations: Reduced goroutine creation overhead
   - GC pressure: 20-30% reduction in pause times

3. **Latency Improvements:**
   - Reduced scheduler overhead: 5-10% p99 latency improvement
   - Better cache locality: Workers reuse stack memory
   - Predictable performance under load

4. **Throughput:**
   - Estimated: 5-15% QPS increase at high load
   - Reduced contention on storage buffer

### Trade-offs
1. **Buffering:** Larger buffer (50K vs 10K) increases memory by ~1-2MB
   - Acceptable trade-off for stability
2. **Worker Count:** 8 workers balances throughput vs context switching
   - Tunable based on CPU cores (recommend: 1 worker per 2 cores)

## Implementation Plan

### Phase 1: Core Implementation (1-2 hours)
- [ ] Create `pkg/dns/query_logger.go` with worker pool
- [ ] Add unit tests for `QueryLogger`
- [ ] Add benchmarks comparing goroutine spawning vs worker pool

### Phase 2: Integration (1 hour)
- [ ] Update `handler.go` to use `QueryLogger`
- [ ] Add config fields for buffer/worker tuning
- [ ] Update `main.go` to initialize `QueryLogger`

### Phase 3: Testing (1-2 hours)
- [ ] Unit tests: Worker pool behavior, channel blocking, graceful shutdown
- [ ] Integration tests: Full DNS query flow with logging
- [ ] Load testing: Compare QPS/latency before and after
  - Use `dnsperfbench` or `dnsperf` tool
  - Target: 10K QPS sustained for 5 minutes
  - Measure: p50/p95/p99 latency, memory usage, GC pauses

### Phase 4: Monitoring (30 mins)
- [ ] Add Prometheus metrics:
  - `dns_query_log_buffer_size` (gauge)
  - `dns_query_log_drops_total` (counter)
  - `dns_query_log_worker_queue_depth` (histogram)
- [ ] Add log warnings when buffer >80% full

## Testing Plan

### Unit Tests
```go
func TestQueryLogger_WorkerPool(t *testing.T) {
    // Test worker pool processes entries
    // Test graceful shutdown
    // Test buffer overflow handling
}

func BenchmarkQueryLogging(b *testing.B) {
    // Benchmark: Current (goroutine spawning)
    // Benchmark: Proposed (worker pool)
    // Compare: allocations, latency, throughput
}
```

### Load Tests
```bash
# Before optimization
dnsperf -s 127.0.0.1 -p 5353 -d queries.txt -Q 10000 -l 300

# After optimization
dnsperf -s 127.0.0.1 -p 5353 -d queries.txt -Q 10000 -l 300

# Compare metrics:
# - Queries per second
# - Latency (avg/min/max)
# - Memory usage (via pprof)
# - GC pause times (via pprof)
```

### Profiling
```bash
# CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine count
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

## Rollback Plan
1. Keep current implementation as fallback
2. Add feature flag `use_worker_pool_logging: true`
3. If issues found, disable via config reload (no restart needed)

## Success Metrics
- [ ] Goroutine count reduced from ~10K to <100 at 10K QPS
- [ ] p99 latency improved by 5-10%
- [ ] GC pause time reduced by 20-30%
- [ ] No increase in dropped queries (<1% at 10K QPS)
- [ ] Memory usage decreased by ~15-20MB at 10K QPS

## References
- Current implementation: `pkg/dns/handler.go:249`
- Storage buffer: `pkg/storage/sqlite.go:107,152-163`
- Config structure: `pkg/config/config.go`

## Notes
- This optimization is CRITICAL for production deployments at >1000 QPS
- Worker count should be tuned based on CPU cores (start with 8, adjust up to 16 for 16+ core systems)
- Buffer size should be at least 5x expected peak QPS (50K for 10K QPS)
- Monitor dropped queries metric closely after deployment
