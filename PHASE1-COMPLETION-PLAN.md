# Phase 1 Completion Plan: Database Logging with Multi-Backend Support

**Goal**: Complete Phase 1 by implementing database logging with support for both local SQLite and Cloudflare D1 storage backends.

**Status**: Phase 1 is 90% complete. This is the final 10%.

---

## Overview

This plan implements a flexible storage layer that supports:
1. **Local SQLite** - For standalone deployments (self-hosted, local networks)
2. **Cloudflare D1** - For cloud deployments (Workers, distributed edge)
3. **Abstraction Layer** - Clean interface allowing easy backend switching

---

## Architecture

### Storage Interface Pattern

```go
// pkg/storage/storage.go

// Storage defines the interface for all storage backends
type Storage interface {
    // Query Logging
    LogQuery(ctx context.Context, query *QueryLog) error
    GetRecentQueries(ctx context.Context, limit int) ([]*QueryLog, error)
    GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*QueryLog, error)

    // Statistics
    GetStatistics(ctx context.Context, since time.Time) (*Statistics, error)
    GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*DomainStats, error)
    GetBlockedCount(ctx context.Context, since time.Time) (int64, error)

    // Maintenance
    Cleanup(ctx context.Context, olderThan time.Time) error
    Close() error
}

// Backend types
type BackendType string

const (
    BackendSQLite BackendType = "sqlite"
    BackendD1     BackendType = "d1"
)

// Factory function
func New(cfg *Config) (Storage, error) {
    switch cfg.Backend {
    case BackendSQLite:
        return NewSQLiteStorage(cfg)
    case BackendD1:
        return NewD1Storage(cfg)
    default:
        return nil, fmt.Errorf("unsupported backend: %s", cfg.Backend)
    }
}
```

---

## Implementation Plan

### Phase 1A: Storage Abstraction Layer (2-3 hours)

**Tasks**:
1. Define `Storage` interface in `pkg/storage/storage.go`
2. Define data models (QueryLog, Statistics, DomainStats)
3. Implement factory pattern for backend selection
4. Add configuration options for backend selection

**Files**:
- `pkg/storage/storage.go` - Interface and models
- `pkg/storage/config.go` - Configuration structs
- `pkg/storage/factory.go` - Backend factory

**Configuration Example**:
```yaml
database:
  backend: "sqlite"  # or "d1"

  # SQLite-specific config
  sqlite:
    path: "./glory-hole.db"
    busy_timeout: 5000

  # D1-specific config
  d1:
    account_id: "your-account-id"
    database_id: "your-database-id"
    api_token: "${D1_API_TOKEN}"

  # Common settings
  retention_days: 7
  batch_size: 100
  flush_interval: "5s"
```

---

### Phase 1B: SQLite Implementation (3-4 hours)

**Tasks**:
1. Implement `SQLiteStorage` struct
2. Create database schema with migrations
3. Implement all Storage interface methods
4. Add connection pooling and prepared statements
5. Implement async write buffer for performance
6. Add retention/cleanup logic

**Schema**:
```sql
-- queries table
CREATE TABLE queries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    client_ip TEXT NOT NULL,
    domain TEXT NOT NULL,
    query_type TEXT NOT NULL,
    response_code INTEGER NOT NULL,
    blocked BOOLEAN NOT NULL,
    cached BOOLEAN NOT NULL,
    response_time_ms REAL NOT NULL,
    upstream TEXT
);

CREATE INDEX idx_queries_timestamp ON queries(timestamp);
CREATE INDEX idx_queries_domain ON queries(domain);
CREATE INDEX idx_queries_blocked ON queries(blocked);
CREATE INDEX idx_queries_client_ip ON queries(client_ip);

-- statistics table (pre-aggregated for performance)
CREATE TABLE statistics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    hour DATETIME NOT NULL,  -- rounded to hour
    total_queries INTEGER NOT NULL,
    blocked_queries INTEGER NOT NULL,
    cached_queries INTEGER NOT NULL,
    avg_response_time_ms REAL NOT NULL,
    unique_domains INTEGER NOT NULL
);

CREATE INDEX idx_statistics_hour ON statistics(hour);

-- domain_stats table (top domains cache)
CREATE TABLE domain_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT NOT NULL,
    query_count INTEGER NOT NULL,
    last_queried DATETIME NOT NULL,
    blocked BOOLEAN NOT NULL
);

CREATE INDEX idx_domain_stats_domain ON domain_stats(domain);
CREATE INDEX idx_domain_stats_count ON domain_stats(query_count DESC);
```

**Files**:
- `pkg/storage/sqlite.go` - SQLite implementation
- `pkg/storage/sqlite_test.go` - Comprehensive tests
- `pkg/storage/migrations/001_initial.sql` - Schema

**Performance Optimizations**:
- Buffered writes (channel-based async inserts)
- Prepared statement caching
- WAL mode for better concurrency
- Batch inserts every 5 seconds or 100 queries
- Hourly statistics pre-aggregation

---

### Phase 1C: D1 Implementation (3-4 hours)

**Tasks**:
1. Implement `D1Storage` struct using Cloudflare D1 HTTP API
2. Implement all Storage interface methods
3. Add HTTP client with retries and circuit breaker
4. Handle D1-specific constraints (1000 queries per invocation)
5. Implement batch operations for efficiency
6. Add comprehensive error handling

**D1 HTTP API Integration**:
```go
// pkg/storage/d1.go

type D1Storage struct {
    accountID  string
    databaseID string
    apiToken   string
    client     *http.Client
    baseURL    string
    buffer     chan *QueryLog
    batchSize  int
}

// Execute SQL via D1 REST API
func (d *D1Storage) execute(ctx context.Context, sql string, params ...interface{}) (*D1Result, error) {
    url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/query",
        d.accountID, d.databaseID)

    body := D1Request{
        SQL:    sql,
        Params: params,
    }

    // Make HTTP request with auth
    // Handle response
    // Parse D1Result
}

// Batch multiple queries
func (d *D1Storage) batch(ctx context.Context, queries []D1Query) ([]D1Result, error) {
    url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/batch",
        d.accountID, d.databaseID)

    // Execute batch (up to 1000 queries)
}
```

**Files**:
- `pkg/storage/d1.go` - D1 implementation
- `pkg/storage/d1_client.go` - HTTP client wrapper
- `pkg/storage/d1_test.go` - Tests with mocked API

**D1 Considerations**:
- Rate limiting: Cloudflare API has global limits
- Batch size: Max 1000 queries per Worker invocation
- Latency: HTTP overhead vs local SQLite
- Cost: Charged per read/write operation
- Best for: Edge deployments, multi-tenant scenarios

**Schema** (same as SQLite):
```sql
-- D1 uses SQLite syntax, so schema is identical
-- Applied via wrangler CLI or HTTP API
```

---

### Phase 1D: DNS Server Integration (2 hours)

**Tasks**:
1. Add storage initialization in `main.go`
2. Integrate logging in `pkg/dns/server.go`
3. Implement async logging to prevent blocking DNS queries
4. Add metrics for database operations
5. Handle storage failures gracefully (log but don't fail DNS)

**Integration Example**:
```go
// cmd/glory-hole/main.go

func main() {
    cfg := config.Load()

    // Initialize storage
    storage, err := storage.New(&cfg.Database)
    if err != nil {
        log.Fatal("Failed to initialize storage:", err)
    }
    defer storage.Close()

    // Initialize DNS server with storage
    dnsServer := dns.NewServer(cfg, cache, blocklistMgr, storage)

    // Start server...
}

// pkg/dns/server.go

func (s *Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
    startTime := time.Now()

    // ... existing DNS handling logic ...

    // Async logging (non-blocking)
    if s.storage != nil {
        go func() {
            ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
            defer cancel()

            queryLog := &storage.QueryLog{
                Timestamp:      startTime,
                ClientIP:       w.RemoteAddr().String(),
                Domain:         domain,
                QueryType:      queryType,
                ResponseCode:   responseCode,
                Blocked:        blocked,
                Cached:         cached,
                ResponseTimeMs: time.Since(startTime).Milliseconds(),
                Upstream:       upstream,
            }

            if err := s.storage.LogQuery(ctx, queryLog); err != nil {
                s.logger.Error("Failed to log query", "error", err)
                // Don't fail DNS request due to logging error
            }
        }()
    }
}
```

**Files**:
- `cmd/glory-hole/main.go` - Storage initialization
- `pkg/dns/server.go` - Async logging integration
- `config.example.yml` - Database config examples

---

### Phase 1E: Testing & Validation (2-3 hours)

**Testing Strategy**:

1. **Unit Tests**:
   - Test each storage backend independently
   - Mock dependencies (HTTP client for D1)
   - Test buffered writes and batching
   - Test error handling and recovery

2. **Integration Tests**:
   - Test real DNS queries → database writes
   - Test concurrent query logging
   - Test retention/cleanup
   - Test statistics aggregation

3. **Performance Tests**:
   - Benchmark SQLite write performance
   - Verify <10µs overhead for async logging
   - Test with 10K+ queries/second
   - Memory usage monitoring

4. **Load Tests**:
   - Sustained load testing (1000 QPS for 1 hour)
   - Database size growth monitoring
   - Cleanup performance validation

**Files**:
- `pkg/storage/sqlite_test.go` - SQLite tests
- `pkg/storage/d1_test.go` - D1 tests
- `pkg/dns/server_integration_test.go` - Integration tests
- `benchmark-database.go` - Performance benchmarks

**Test Coverage Goals**:
- Storage interface: 100%
- SQLite implementation: 95%+
- D1 implementation: 90%+
- Integration: 85%+

---

## Configuration

### Example Config (SQLite)
```yaml
database:
  enabled: true
  backend: "sqlite"

  sqlite:
    path: "./glory-hole.db"
    busy_timeout: 5000
    wal_mode: true
    cache_size: 10000

  buffer_size: 1000
  flush_interval: "5s"
  retention_days: 7

  statistics:
    enabled: true
    aggregation_interval: "1h"
```

### Example Config (D1)
```yaml
database:
  enabled: true
  backend: "d1"

  d1:
    account_id: "${CF_ACCOUNT_ID}"
    database_id: "${D1_DATABASE_ID}"
    api_token: "${CF_API_TOKEN}"

  buffer_size: 500
  flush_interval: "10s"
  retention_days: 30

  statistics:
    enabled: true
    aggregation_interval: "1h"
```

---

## Performance Targets

### SQLite Backend
- Query logging overhead: **<10µs** (async buffered)
- Write throughput: **>10,000 inserts/second** (batched)
- Query response time: **<1ms** for statistics
- Database size: **~1MB per 10,000 queries**
- Memory overhead: **<50MB** for buffer

### D1 Backend
- Query logging overhead: **<10µs** (async buffered)
- HTTP latency: **~50-100ms per batch** (edge network)
- Batch efficiency: **500-1000 queries per batch**
- Cost efficiency: **Minimal reads, batched writes**

### Both Backends
- No DNS query failures due to logging
- Graceful degradation on storage errors
- Automatic retry with exponential backoff
- Circuit breaker for failing backends

---

## Migration Strategy

### For Existing Deployments
1. Database is optional (enabled: false)
2. Default to SQLite for simplicity
3. Provide migration tool for SQLite → D1
4. Document performance characteristics

### For New Deployments
- **Self-hosted**: Use SQLite (zero dependencies)
- **Cloudflare Workers**: Use D1 (native integration)
- **Hybrid**: Use SQLite with D1 backup/sync

---

## Success Criteria

### Functional Requirements
- ✅ All DNS queries logged to database
- ✅ Statistics accurately calculated
- ✅ Retention policy working
- ✅ Both SQLite and D1 backends functional
- ✅ Graceful error handling

### Performance Requirements
- ✅ <10µs overhead per query (async)
- ✅ >10,000 writes/second (SQLite)
- ✅ No DNS failures due to logging
- ✅ Memory usage <100MB

### Quality Requirements
- ✅ 100% test coverage on storage interface
- ✅ 90%+ coverage on implementations
- ✅ Comprehensive integration tests
- ✅ Load tests passing (1000 QPS sustained)

---

## Estimated Timeline

| Task | Duration | Dependencies |
|------|----------|--------------|
| 1A. Storage Abstraction | 2-3 hours | None |
| 1B. SQLite Implementation | 3-4 hours | 1A |
| 1C. D1 Implementation | 3-4 hours | 1A |
| 1D. DNS Integration | 2 hours | 1B or 1C |
| 1E. Testing & Validation | 2-3 hours | 1D |
| **Total** | **12-16 hours** | **~2 days** |

---

## Implementation Order

### Day 1: Foundation & SQLite
1. Morning: Storage abstraction layer (1A)
2. Afternoon: SQLite implementation (1B)
3. Evening: SQLite testing

### Day 2: D1 & Integration
1. Morning: D1 implementation (1C)
2. Afternoon: DNS integration (1D)
3. Evening: Full testing & validation (1E)

---

## Future Enhancements (Post Phase 1)

### Phase 2 Improvements
- Real-time statistics dashboard
- Query log streaming (WebSocket)
- Advanced analytics (query patterns, anomaly detection)
- Export to external analytics (Prometheus, Grafana)

### Multi-Backend Sync
- SQLite → D1 replication
- Multi-region D1 databases
- Sharding for high-volume deployments

### Advanced Features
- Full-text search on domains
- Query pattern analysis
- Machine learning for anomaly detection
- Integration with external SIEM systems

---

## Documentation Tasks

- [ ] Update STATUS.md (Phase 1 → 100%)
- [ ] Create DATABASE-DESIGN.md
- [ ] Update ARCHITECTURE.md (add storage layer)
- [ ] Update README.md (database configuration)
- [ ] Create D1-INTEGRATION-GUIDE.md
- [ ] Update config.example.yml
- [ ] Add migration guide (SQLite ↔ D1)

---

## Risk Mitigation

### Risk 1: SQLite Performance
- **Mitigation**: Buffered writes, WAL mode, prepared statements
- **Fallback**: Disable logging temporarily under extreme load

### Risk 2: D1 API Limits
- **Mitigation**: Batch operations, rate limiting, exponential backoff
- **Fallback**: Queue to local disk, sync later

### Risk 3: Database Corruption
- **Mitigation**: WAL mode, fsync, backup jobs, Time Travel (D1)
- **Fallback**: Rebuild from recent backups

### Risk 4: Memory Growth
- **Mitigation**: Bounded buffers, flow control, monitoring
- **Fallback**: Flush buffers aggressively

---

## Next Steps

1. **Review this plan** with stakeholders
2. **Approve backend selection** (SQLite only? Both?)
3. **Begin implementation** starting with 1A
4. **Iterate and test** continuously
5. **Document as you go**

---

**Last Updated**: 2025-11-21
**Author**: Glory-Hole Development Team
**Status**: Ready for implementation
