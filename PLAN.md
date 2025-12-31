# Performance Optimization: Storage Buffer Management

## Issue Classification
**Priority:** P1 - CRITICAL
**Category:** Performance - Data Loss Prevention
**Impact:** High traffic environments (>5K QPS)

## Problem Statement

### Current Implementation
Location: `pkg/storage/sqlite.go:107,152-163`

```go
func NewSQLiteStorage(cfg *Config, logger *slog.Logger) (*SQLiteStorage, error) {
    // ...
    queryBuffer := make(chan *QueryEntry, cfg.BufferSize) // Default: 10K
    // ...
}

func (s *SQLiteStorage) LogQuery(ctx context.Context, entry *QueryEntry) error {
    select {
    case s.queryBuffer <- entry:
        return nil
    default:
        // Buffer full - drop query silently
        s.AddDroppedQuery()
        return ErrBufferFull
    }
}
```

### Root Cause Analysis
1. **Fixed Buffer Size:** Default 10K buffer exhausted at sustained 10K+ QPS
2. **Silent Drops:** Queries dropped when buffer full, only metric tracked
3. **Flush Latency:** If SQLite write slower than intake, buffer saturates
4. **No Backpressure:** DNS handler doesn't slow down when storage overwhelmed

### Impact Assessment
1. **Data Loss:** 1-5% query loss at peak traffic
2. **Analytics Gaps:** Missing data for billing, compliance, debugging
3. **Cascading Issues:** Dropped queries hide actual traffic patterns

## Proposed Solution

### Multi-Layered Approach

#### 1. Increase Default Buffer Size
**Rationale:** Absorb traffic spikes without drops

```diff
type Config struct {
-   BufferSize int `json:"buffer_size" default:"10000"`
+   BufferSize int `json:"buffer_size" default:"50000"`
}
```

**Impact:**
- Memory increase: ~2MB for 40K additional entries
- Buffer can absorb 5-second spike at 10K QPS

#### 2. Add Dynamic Buffer Monitoring
**File:** `pkg/storage/sqlite.go`

```go
type SQLiteStorage struct {
    // ... existing fields
    bufferHighWatermark int
    bufferWarningLogged atomic.Bool
}

func (s *SQLiteStorage) LogQuery(ctx context.Context, entry *QueryEntry) error {
    currentSize := len(s.queryBuffer)

    // Warn when buffer >80% full
    if currentSize > s.bufferHighWatermark {
        if s.bufferWarningLogged.CompareAndSwap(false, true) {
            s.logger.Warn("Query buffer high watermark exceeded",
                "current", currentSize,
                "capacity", cap(s.queryBuffer),
                "utilization", fmt.Sprintf("%.1f%%", float64(currentSize)/float64(cap(s.queryBuffer))*100))
        }
    } else if currentSize < s.bufferHighWatermark/2 {
        // Reset warning flag when buffer drains
        s.bufferWarningLogged.Store(false)
    }

    select {
    case s.queryBuffer <- entry:
        return nil
    default:
        s.AddDroppedQuery()
        s.logger.Error("Query buffer full - dropping entry",
            "domain", entry.Domain,
            "client_ip", entry.ClientIP)
        return ErrBufferFull
    }
}
```

#### 3. Optimize Flush Performance
**Current:** `flushWorker()` batches queries but may have tuning issues

**Analysis Needed:**
- Check batch size configuration (default: 100)
- Verify SQLite PRAGMA settings
- Monitor transaction commit times

**File:** `pkg/storage/sqlite.go:246`

```go
func (s *SQLiteStorage) flushWorker() {
    ticker := time.NewTicker(s.flushInterval)
    defer ticker.Stop()

    batch := make([]*QueryEntry, 0, s.batchSize)

    for {
        select {
        case <-s.ctx.Done():
            s.flushBatch(batch)
            return

        case entry := <-s.queryBuffer:
            batch = append(batch, entry)

            // Flush when batch size reached
            if len(batch) >= s.batchSize {
                s.flushBatch(batch)
                batch = batch[:0] // Reset slice but keep capacity
            }

        case <-ticker.C:
            // Periodic flush of partial batches
            if len(batch) > 0 {
                s.flushBatch(batch)
                batch = batch[:0]
            }
        }
    }
}

func (s *SQLiteStorage) flushBatch(batch []*QueryEntry) {
+   startTime := time.Now()

    tx, err := s.db.Begin()
    if err != nil {
        s.logger.Error("Failed to begin transaction", "error", err)
        return
    }

    for _, entry := range batch {
        // ... existing insert logic
    }

    if err := tx.Commit(); err != nil {
        s.logger.Error("Failed to commit batch", "error", err, "batch_size", len(batch))
        tx.Rollback()
        return
    }

+   flushDuration := time.Since(startTime)
+   s.logger.Debug("Flushed batch",
+       "batch_size", len(batch),
+       "duration_ms", flushDuration.Milliseconds())
+
+   // Alert if flush taking too long
+   if flushDuration > 100*time.Millisecond {
+       s.logger.Warn("Slow batch flush detected",
+           "batch_size", len(batch),
+           "duration_ms", flushDuration.Milliseconds())
+   }
}
```

#### 4. Add Prometheus Metrics
**File:** `pkg/storage/metrics.go` (new file)

```go
var (
    queryBufferSize = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "gloryhole_query_buffer_size",
            Help: "Current size of query log buffer",
        },
        []string{"status"},
    )

    queryBufferCapacity = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "gloryhole_query_buffer_capacity",
            Help: "Maximum capacity of query log buffer",
        },
    )

    batchFlushDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name: "gloryhole_batch_flush_duration_seconds",
            Help: "Duration of batch flush operations",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to 1s
        },
    )

    batchFlushSize = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name: "gloryhole_batch_flush_size",
            Help: "Number of queries in batch flush",
            Buckets: prometheus.LinearBuckets(0, 10, 20), // 0 to 200
        },
    )
)

func (s *SQLiteStorage) updateBufferMetrics() {
    queryBufferSize.WithLabelValues("used").Set(float64(len(s.queryBuffer)))
    queryBufferCapacity.Set(float64(cap(s.queryBuffer)))
}
```

#### 5. Implement Adaptive Flushing
**Concept:** Adjust flush interval based on buffer utilization

```go
func (s *SQLiteStorage) adaptiveFlushWorker() {
    minInterval := 100 * time.Millisecond
    maxInterval := 5 * time.Second
    currentInterval := s.flushInterval

    ticker := time.NewTicker(currentInterval)
    defer ticker.Stop()

    batch := make([]*QueryEntry, 0, s.batchSize)

    for {
        bufferUtil := float64(len(s.queryBuffer)) / float64(cap(s.queryBuffer))

        // Adjust flush frequency based on buffer utilization
        if bufferUtil > 0.8 {
            // Buffer filling fast - flush more frequently
            currentInterval = minInterval
        } else if bufferUtil < 0.2 {
            // Buffer mostly empty - flush less frequently
            currentInterval = maxInterval
        } else {
            // Normal operation
            currentInterval = s.flushInterval
        }

        ticker.Reset(currentInterval)

        // ... existing flush logic
    }
}
```

## Expected Impact

### Performance Improvements
1. **Reduced Data Loss:**
   - Before: 1-5% query loss at >10K QPS
   - After: <0.1% query loss at 10K QPS
   - Impact: 95-98% reduction in dropped queries

2. **Buffer Headroom:**
   - Before: 10K buffer = 1 second at 10K QPS
   - After: 50K buffer = 5 seconds at 10K QPS
   - Impact: 5x spike absorption capacity

3. **Operational Visibility:**
   - Real-time buffer utilization metrics
   - Early warning (80% watermark) before drops
   - Flush performance monitoring

4. **Adaptive Performance:**
   - Flush rate adjusts to load
   - Reduced SQLite contention during low traffic
   - Faster flushing during high traffic

### Memory Trade-offs
- Additional memory: ~2MB for larger buffer
- Acceptable for improved reliability
- Configurable via `buffer_size` parameter

## Implementation Plan

### Phase 1: Buffer Size Increase (30 mins)
- [ ] Update default `BufferSize` from 10K to 50K
- [ ] Add configuration validation (min: 10K, max: 200K)
- [ ] Update documentation

### Phase 2: Monitoring & Alerting (1-2 hours)
- [ ] Add buffer watermark detection
- [ ] Implement buffer utilization logging
- [ ] Add Prometheus metrics for buffer size/utilization
- [ ] Add batch flush duration metrics

### Phase 3: Flush Optimization (2-3 hours)
- [ ] Add flush duration logging
- [ ] Profile slow flushes (>100ms)
- [ ] Implement adaptive flush intervals
- [ ] Optimize SQLite PRAGMA settings if needed

### Phase 4: Testing (1-2 hours)
- [ ] Unit tests: Buffer overflow behavior
- [ ] Load tests: Sustained 10K QPS for 10 minutes
- [ ] Verify dropped query rate <0.1%
- [ ] Monitor buffer utilization metrics

### Phase 5: Documentation (30 mins)
- [ ] Update config documentation
- [ ] Add operational runbook for buffer tuning
- [ ] Document Prometheus alerts

## Testing Plan

### Unit Tests
```go
func TestBufferOverflow(t *testing.T) {
    // Create storage with small buffer
    cfg := &Config{BufferSize: 10}
    storage, _ := NewSQLiteStorage(cfg, logger)

    // Fill buffer beyond capacity
    for i := 0; i < 20; i++ {
        entry := &QueryEntry{Domain: fmt.Sprintf("test%d.com", i)}
        err := storage.LogQuery(context.Background(), entry)

        if i < 10 {
            assert.NoError(t, err)
        } else {
            assert.Equal(t, ErrBufferFull, err)
        }
    }

    // Verify dropped count
    assert.Equal(t, int64(10), storage.GetDroppedQueries())
}

func TestAdaptiveFlushing(t *testing.T) {
    // Test flush interval adjusts based on buffer utilization
}
```

### Load Tests
```bash
# Generate 10K QPS sustained load
dnsperf -s 127.0.0.1 -p 5353 -d queries.txt -Q 10000 -l 600

# Monitor metrics during test
watch -n 1 'curl -s http://localhost:9090/metrics | grep query_buffer'

# Expected results:
# - gloryhole_query_buffer_size < 40000 (80% of 50K)
# - gloryhole_dropped_queries_total < 600 (0.1% of 6M queries)
```

### Profiling
```bash
# Monitor SQLite write performance
sqlite3 glory-hole.db "PRAGMA wal_checkpoint;"
sqlite3 glory-hole.db "PRAGMA optimize;"

# Check for lock contention
sqlite3 glory-hole.db "PRAGMA busy_timeout;"
```

## Rollback Plan
1. Configuration change only - can revert via config
2. If issues found, decrease `buffer_size` to 10K
3. Disable adaptive flushing via feature flag

## Success Metrics
- [ ] Dropped query rate <0.1% at 10K QPS sustained
- [ ] Buffer never exceeds 90% utilization
- [ ] Batch flush duration <50ms p99
- [ ] No SQLite lock timeout errors
- [ ] Alert fires when buffer >80% for >60 seconds

## Monitoring & Alerting

### Prometheus Alerts
```yaml
groups:
- name: gloryhole_storage
  rules:
  - alert: QueryBufferHighUtilization
    expr: gloryhole_query_buffer_size / gloryhole_query_buffer_capacity > 0.8
    for: 1m
    annotations:
      summary: "Query buffer high utilization"
      description: "Buffer is {{ $value | humanizePercentage }} full"

  - alert: QueryBufferDropping
    expr: rate(gloryhole_dropped_queries_total[5m]) > 1
    for: 2m
    annotations:
      summary: "Queries being dropped"
      description: "{{ $value }} queries/sec dropped"

  - alert: SlowBatchFlush
    expr: histogram_quantile(0.99, rate(gloryhole_batch_flush_duration_seconds_bucket[5m])) > 0.1
    for: 5m
    annotations:
      summary: "Slow batch flush detected"
      description: "p99 flush duration is {{ $value }}s"
```

## References
- Current implementation: `pkg/storage/sqlite.go:107,152-163`
- Flush worker: `pkg/storage/sqlite.go:246`
- Configuration: `pkg/config/config.go`

## Notes
- This optimization prevents data loss during traffic spikes
- Buffer size should be tuned based on:
  - Peak QPS × burst duration (seconds)
  - Example: 10K QPS × 5s = 50K buffer
- Monitor dropped queries metric closely after deployment
- Consider adding disk space alerts (WAL growth)
