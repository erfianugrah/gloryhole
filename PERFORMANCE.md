# Performance Optimization Plan

## Current Performance
- **Direct upstream query**: ~4ms
- **Through Glory Hole**: ~8ms
- **Overhead**: ~4ms (100% increase)

## Hot Path Analysis

### Current Request Flow
1. `wrappedHandler.serveDNS()` - Logging, metrics (line pkg/dns/server_impl.go:159)
2. `Handler.ServeDNS()` - Core logic (line pkg/dns/server.go:41)
   - Validate request
   - RWMutex lock for whitelist check
   - RWMutex lock for blocklist check
   - RWMutex lock for overrides check
   - RWMutex lock for CNAME overrides check
   - Forward to upstream
3. `Forwarder.Forward()` - Upstream query (line pkg/forwarder/forwarder.go:67)
   - Get client from sync.Pool
   - Atomic operation for round-robin
   - DNS exchange
   - Return client to pool

### Bottlenecks Identified

#### 1. Multiple RWMutex Locks (Biggest Impact)
**Current**: 4 separate RWMutex.RLock() calls per query
```go
h.WhitelistMu.RLock()
_, whitelisted := h.Whitelist[domain]
h.WhitelistMu.RUnlock()

h.BlocklistMu.RLock()
_, blocked := h.Blocklist[domain]
h.BlocklistMu.RUnlock()

h.OverridesMu.RLock()
ip, found := h.Overrides[domain]
h.OverridesMu.RUnlock()

h.CNAMEOverridesMu.RLock()
target, found := h.CNAMEOverrides[domain]
h.CNAMEOverridesMu.RUnlock()
```

**Impact**: ~500-1000ns per lock/unlock = ~2-4μs total
**Solution**: Single RWMutex for all maps OR atomic.Value with copy-on-write

#### 2. Debug Logging Overhead
**Current**: `AddSource: true` adds ~1-2μs per log line
```go
opts := &slog.HandlerOptions{
    Level: level,
    AddSource: level == slog.LevelDebug,  // Expensive!
}
```

**Impact**: ~2-4μs per request (4 log lines)
**Solution**: Disable AddSource in production, use info level

#### 3. Metrics Recording
**Current**: Multiple metrics calls per request
```go
w.metrics.DNSQueriesTotal.Add(ctx, 1)
w.metrics.DNSQueriesByType.Add(ctx, 1)
w.metrics.DNSQueryDuration.Record(ctx, float64(duration.Milliseconds()))
```

**Impact**: ~1-2μs per metric
**Solution**: Batch metrics updates, use fast-path for common cases

#### 4. Memory Allocations
**Current**: Multiple allocations in hot path
- `new(dns.Msg)` for response
- Context creation
- String conversions for logging

**Impact**: ~1μs + GC pressure
**Solution**: Object pooling for dns.Msg

## Optimization Strategy

### Phase 1: Low-Hanging Fruit (Quick Wins)

#### 1.1 Configure Logging Source Location
```yaml
logging:
  level: "debug"
  add_source: true   # Enable for debugging (adds ~1-2μs per log)
  # OR
  add_source: false  # Disable for maximum performance
```
**Performance impact**:
- `add_source: true` → +2-3ms overhead (great for debugging)
- `add_source: false` → minimal overhead (production mode)

#### 1.2 Use Info Level for Production (Optional)
```yaml
logging:
  level: "info"  # Skip debug logs
```
**Expected gain**: Skip 2-3 log lines per query (optional optimization)

#### 1.3 Optimize Metrics (Optional Flag)
```go
// Add fast-path option to skip metrics
if w.metrics != nil && cfg.DetailedMetrics {
    // Record metrics
}
```
**Expected gain**: 1-2ms

### Phase 2: Lock Optimization (Bigger Impact)

#### 2.1 Single RWMutex for All Maps
```go
type Handler struct {
    // Single lock for all lookup maps
    lookupMu sync.RWMutex

    Blocklist      map[string]struct{}
    Whitelist      map[string]struct{}
    Overrides      map[string]net.IP
    CNAMEOverrides map[string]string
}

func (h *Handler) lookup(domain string, qtype uint16) (result, bool) {
    h.lookupMu.RLock()
    defer h.lookupMu.RUnlock()

    // All lookups under single lock
    if _, whitelisted := h.Whitelist[domain]; whitelisted {
        return resultWhitelisted, true
    }
    if _, blocked := h.Blocklist[domain]; blocked {
        return resultBlocked, true
    }
    // ... etc
}
```
**Expected gain**: 4 locks → 1 lock = ~1-2ms

#### 2.2 Atomic.Value with Copy-on-Write (Advanced)
```go
type lookupMaps struct {
    blocklist      map[string]struct{}
    whitelist      map[string]struct{}
    overrides      map[string]net.IP
    cnameOverrides map[string]string
}

type Handler struct {
    maps atomic.Value  // *lookupMaps - No locks for reads!
}

func (h *Handler) ServeDNS(...) {
    maps := h.maps.Load().(*lookupMaps)
    // No locks needed for reads!
    _, blocked := maps.blocklist[domain]
}
```
**Expected gain**: Lock-free reads = ~2-3ms

### Phase 3: Connection Optimization

#### 3.1 Pre-allocated UDP Connections
```go
type Forwarder struct {
    conns []*net.UDPConn  // Pre-allocated connections
}
```
**Expected gain**: Skip connection setup = ~500ns

#### 3.2 Reuse dns.Msg Objects
```go
var msgPool = sync.Pool{
    New: func() interface{} {
        return new(dns.Msg)
    },
}

func (h *Handler) ServeDNS(...) {
    msg := msgPool.Get().(*dns.Msg)
    defer msgPool.Put(msg)
    msg.SetReply(r)
    // ...
}
```
**Expected gain**: Reduce allocations = ~500ns + less GC

### Phase 4: Build Optimization

#### 4.1 Compiler Optimizations
```bash
go build -ldflags="-s -w" -gcflags="-l=4" -o glory-hole ./cmd/glory-hole/
```
**Expected gain**: ~500ns

#### 4.2 Profile-Guided Optimization (PGO)
```bash
# 1. Build with profiling
go build -o glory-hole ./cmd/glory-hole/

# 2. Run with profiling
./glory-hole -cpuprofile=cpu.prof

# 3. Rebuild with profile
go build -pgo=cpu.prof -o glory-hole ./cmd/glory-hole/
```
**Expected gain**: 5-15% overall

## Implementation Priority

### Immediate (Today)
1. ✅ Make AddSource configurable in logging (0-3ms gain depending on setting)
2. ✅ Single RWMutex for all maps (1-2ms gain)
3. ⬜ Use info-level logging for production (optional, 1-2ms gain)

**Expected total**: 8ms → 5-6ms (25-40% reduction)
**With add_source=false**: 8ms → 3-4ms (50% reduction)

### Short-term (This Week)
4. ⬜ atomic.Value for lock-free reads (2-3ms gain)
5. ⬜ dns.Msg pooling (500ns gain)
6. ⬜ Build optimizations (500ns gain)

**Expected total**: 8ms → 2-3ms (70% reduction)

### Long-term (Future Optimization)
7. ⬜ Pre-allocated connections
8. ⬜ Profile-guided optimization
9. ⬜ Custom DNS parser (extreme optimization)
10. ⬜ DPDK/io_uring for kernel bypass (extreme optimization)

**Expected total**: 8ms → 1-2ms (close to direct upstream)

## Benchmarking Plan

Create performance benchmarks:
```bash
# Create benchmark
go test -bench=BenchmarkServeDNS -benchmem ./pkg/dns/

# Before optimization
BenchmarkServeDNS-8    50000    25000 ns/op    2048 B/op    20 allocs/op

# After optimization goal
BenchmarkServeDNS-8    100000   10000 ns/op    512 B/op     5 allocs/op
```

## Monitoring

Add performance metrics:
- `dns.query.duration` - p50, p95, p99
- `dns.upstream.duration` - Upstream RTT
- `dns.overhead.duration` - Our overhead
- `dns.locks.contention` - Lock wait time

## Notes

- 8ms total is already good for DNS
- Most of overhead is debug logging (removable)
- Lock contention is minimal with current load
- Optimize based on actual load patterns
- Don't over-optimize before measuring
