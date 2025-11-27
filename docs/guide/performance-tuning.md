# Performance Tuning Guide

This guide covers performance optimization techniques for Glory-Hole DNS Server.

## Table of Contents

- [Cache Sharding](#cache-sharding)
- [Database Optimization](#database-optimization)
- [Memory Management](#memory-management)
- [Rate Limiting](#rate-limiting)
- [Monitoring](#monitoring)

## Cache Sharding

**Added in:** Phase 2 Performance Optimization

### Overview

Cache sharding distributes cache entries across multiple independent shards, each with its own lock. This dramatically reduces lock contention under high concurrent load, allowing better utilization of multi-core CPUs.

### When to Enable

| Scenario | Recommended Setting | Expected Benefit |
|----------|-------------------|------------------|
| Low traffic (<10K QPS) | `shard_count: 0` (disabled) | No overhead |
| Medium traffic (10-100K QPS) | `shard_count: 16` | 20-30% improvement |
| High traffic (>100K QPS) | `shard_count: 64` | 30-50% improvement |
| Very high traffic (>500K QPS) | `shard_count: 128` | 40-60% improvement |

### Configuration

```yaml
cache:
  enabled: true
  max_entries: 10000
  shard_count: 64  # Enable sharding with 64 shards
```

**Important Notes:**
- `shard_count: 0` (default) uses traditional non-sharded cache (backward compatible)
- Shard count should generally be a power of 2 (16, 32, 64, 128)
- Each shard gets approximately `max_entries / shard_count` capacity
- Memory overhead is negligible (<1% per shard)

### Architecture

```
┌─────────────────────────────────────────┐
│         Incoming DNS Queries            │
└──────────────┬──────────────────────────┘
               │
               ▼
         ┌─────────────┐
         │ FNV-1a Hash │  (Fast domain hashing)
         └──────┬──────┘
                │
    ┌───────────┴───────────┐
    │                       │
    ▼                       ▼
┌────────┐             ┌────────┐
│ Shard 0│             │ Shard N│
│ + Lock │   ...       │ + Lock │
└────────┘             └────────┘

Each shard operates independently,
allowing parallel cache operations
```

### Performance Metrics

Real-world benchmarks on 8-core AMD Ryzen 7 7800X3D:

| Configuration | QPS | P99 Latency | Lock Contention |
|--------------|-----|-------------|-----------------|
| Non-sharded (0) | 240K | 0.8ms | High (35%) |
| 16 shards | 310K | 0.6ms | Medium (20%) |
| 64 shards | 380K | 0.4ms | Low (8%) |
| 128 shards | 390K | 0.4ms | Very Low (5%) |

**Diminishing Returns:** Beyond 64 shards, improvements are minimal on typical hardware.

### Best Practices

1. **Start Conservative:** Begin with `shard_count: 0` and enable only if needed
2. **Monitor Lock Contention:** Use Prometheus metrics to track cache performance
3. **Match Core Count:** A good starting point is 4-8× your CPU core count
4. **Test Under Load:** Benchmark with realistic traffic patterns before deploying

### Troubleshooting

**Problem:** No performance improvement after enabling sharding
**Solution:** Your traffic may not have enough concurrency. Monitor cache hit rate and concurrent requests.

**Problem:** Increased memory usage
**Solution:** Reduce `max_entries` proportionally when enabling sharding.

## Database Optimization

### WAL Mode (Enabled by Default)

Write-Ahead Logging (WAL) provides better concurrency for SQLite:

```yaml
database:
  backend: "sqlite"
  sqlite:
    wal_mode: true  # Enabled by default
    busy_timeout: 5000
    cache_size: 4096
```

**Benefits:**
- Readers don't block writers
- Writers don't block readers
- Better performance under concurrent load

### Batching and Buffering

Reduce database writes by batching:

```yaml
database:
  buffer_size: 1000      # Buffer queries before writing
  flush_interval: "5s"   # Flush buffer every 5 seconds
  batch_size: 100        # Write in batches of 100
```

**Trade-off:** Higher buffering = less disk I/O but potential data loss on crash.

### Retention Policy

Limit database growth with automatic cleanup:

```yaml
database:
  retention_days: 7  # Keep only last 7 days
```

## Memory Management

### Message Pool

**Automatically enabled in Phase 1 optimization**

DNS messages are pooled and reused, reducing allocations by 35-50%:

- 152-232 bytes saved per query
- At 1M QPS: Saves ~120-160 MB/sec allocations
- Zero configuration required (enabled automatically)

### Cache Size Tuning

Calculate optimal cache size:

```
Recommended max_entries = (Expected unique domains per hour) × 1.5
```

Examples:
- Small network (100 devices): 5,000-10,000
- Medium network (1000 devices): 25,000-50,000
- Large network (10,000+ devices): 100,000-500,000

**Memory footprint:** ~100-200 bytes per entry (varies by domain length)

## Rate Limiting

### Per-Client Limits

Protect against abuse with client-specific rate limiting:

```yaml
rate_limit:
  enabled: true
  requests_per_second: 100
  burst: 200
  on_exceed: "nxdomain"  # or "drop"
  max_tracked_clients: 10000
```

### Override Rules

Customize limits for specific clients or networks:

```yaml
rate_limit:
  overrides:
    - name: "iot-network"
      cidrs:
        - "192.168.10.0/24"
      requests_per_second: 5
      burst: 10
```

**Cost:** ~256 bytes per tracked client (automatically cleaned up)

## Monitoring

### Key Metrics

Monitor these Prometheus metrics:

**Cache Performance:**
- `dns_cache_hits_total` / `dns_cache_misses_total` → Hit rate
- `dns_cache_size` → Current entries
- Lock contention (check with profiling)

**Query Performance:**
- `dns_queries_total` → Total QPS
- `dns_query_duration_seconds` → Latency percentiles
- `dns_blocked_queries_total` → Block rate

**Resource Usage:**
- `process_resident_memory_bytes` → Memory usage
- `go_goroutines` → Goroutine count (should be stable)

### Performance Profiling

Enable pprof for live profiling:

```bash
# CPU profiling
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof

# Memory profiling
curl http://localhost:8080/debug/pprof/heap > mem.prof
go tool pprof mem.prof

# Goroutine profiling
curl http://localhost:8080/debug/pprof/goroutine > goroutine.prof
```

### Load Testing

Test your configuration with realistic traffic:

```bash
# Built-in load test (30-second sustained load)
go test ./test/load -v -run TestDNSLoadSustained

# Custom query patterns
go test ./test/load -v -run TestDNSMemoryProfile
```

## Recommended Configurations

### Small Home Network (<100 devices)

```yaml
cache:
  enabled: true
  max_entries: 5000
  shard_count: 0  # Sharding not needed

database:
  buffer_size: 100
  retention_days: 7

rate_limit:
  enabled: false  # Not needed for home use
```

**Expected Performance:** 5-10K QPS, <1ms P99 latency

### Medium Office Network (100-1000 devices)

```yaml
cache:
  enabled: true
  max_entries: 25000
  shard_count: 16  # Light sharding

database:
  buffer_size: 500
  retention_days: 14

rate_limit:
  enabled: true
  requests_per_second: 50
```

**Expected Performance:** 50-100K QPS, <2ms P99 latency

### Large Enterprise Network (1000+ devices)

```yaml
cache:
  enabled: true
  max_entries: 100000
  shard_count: 64  # Full sharding

database:
  buffer_size: 2000
  retention_days: 30
  batch_size: 200

rate_limit:
  enabled: true
  requests_per_second: 100
  max_tracked_clients: 50000
```

**Expected Performance:** 200-500K QPS, <1ms P99 latency

### High-Traffic ISP/CDN (10,000+ devices)

```yaml
cache:
  enabled: true
  max_entries: 500000
  shard_count: 128  # Maximum sharding

database:
  buffer_size: 5000
  batch_size: 500
  retention_days: 7  # Reduce storage

rate_limit:
  enabled: true
  requests_per_second: 200
  max_tracked_clients: 100000
```

**Expected Performance:** 500K-1M+ QPS, <1ms P99 latency

## Optimization Checklist

- [ ] Enable cache sharding for >10K QPS deployments
- [ ] Configure appropriate `max_entries` based on query patterns
- [ ] Enable WAL mode for SQLite (enabled by default)
- [ ] Set reasonable retention policies
- [ ] Configure rate limiting for untrusted networks
- [ ] Monitor cache hit rates (target >70%)
- [ ] Profile under realistic load
- [ ] Tune shard count based on metrics

## See Also

- [Configuration Guide](configuration.md)
- [Monitoring Guide](monitoring.md)
- [Troubleshooting Guide](troubleshooting.md)
- [Architecture Documentation](../architecture/)
