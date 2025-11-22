# Glory-Hole Component Architecture

**Last Updated:** 2025-11-22
**Version:** 0.6.0
**Status:** Production

This document provides detailed documentation for each component in the Glory-Hole DNS server, including their purpose, key interfaces, thread-safety patterns, and performance characteristics.

---

## Table of Contents

1. [Overview](#overview)
2. [DNS Package](#dns-package)
3. [Blocklist Package](#blocklist-package)
4. [Cache Package](#cache-package)
5. [Forwarder Package](#forwarder-package)
6. [Storage Package](#storage-package)
7. [Policy Package](#policy-package)
8. [API Package](#api-package)
9. [Config Package](#config-package)
10. [Logging Package](#logging-package)
11. [Telemetry Package](#telemetry-package)
12. [Local Records Package](#localrecords-package)
13. [Component Interactions](#component-interactions)

---

## Overview

Glory-Hole is built using a modular architecture with clear separation of concerns. Each package is designed to be:

- **Self-contained**: Minimal dependencies on other packages
- **Thread-safe**: Safe for concurrent access
- **Testable**: Interfaces and dependency injection
- **Performant**: Optimized for high throughput and low latency

### Component Hierarchy

```
┌─────────────────────────────────────────┐
│              API Server                  │
│         (REST API + Web UI)              │
└─────────────────┬───────────────────────┘
                  │
┌─────────────────▼───────────────────────┐
│            DNS Handler                   │
│   (Request Processing & Coordination)    │
└─┬──┬──┬──┬──┬──┬──┬─────────────────────┘
  │  │  │  │  │  │  │
  │  │  │  │  │  │  └──────► Telemetry
  │  │  │  │  │  │              (Metrics)
  │  │  │  │  │  │
  │  │  │  │  │  └─────────► Storage
  │  │  │  │  │                (Query Logging)
  │  │  │  │  │
  │  │  │  │  └────────────► Forwarder
  │  │  │  │                   (Upstream DNS)
  │  │  │  │
  │  │  │  └───────────────► Policy Engine
  │  │  │                      (Rule Evaluation)
  │  │  │
  │  │  └──────────────────► Local Records
  │  │                         (A/AAAA/CNAME)
  │  │
  │  └─────────────────────► Cache
  │                            (Response Caching)
  │
  └────────────────────────► Blocklist Manager
                               (Domain Filtering)
```

---

## DNS Package

**Location:** `/home/erfi/gloryhole/pkg/dns`

### Purpose

The DNS package is the core of Glory-Hole, responsible for:
- Handling DNS queries over UDP and TCP
- Coordinating all other components
- Processing requests through filtering pipeline
- Managing DNS protocol details

### Key Types

#### Handler

The main DNS request handler that implements `dns.Handler` interface:

```go
type Handler struct {
    // Single lock for all lookup maps (performance optimization)
    lookupMu sync.RWMutex

    // Blocklist manager with lock-free updates (FAST PATH)
    BlocklistManager *blocklist.Manager

    // Legacy static maps (SLOW PATH - backward compatibility)
    Blocklist      map[string]struct{}
    Whitelist      map[string]struct{}
    Overrides      map[string]net.IP
    CNAMEOverrides map[string]string

    // Component dependencies
    LocalRecords *localrecords.Manager
    PolicyEngine *policy.Engine
    Forwarder    *forwarder.Forwarder
    Cache        *cache.Cache
    Storage      storage.Storage
}
```

**Responsibilities:**
- Receive and validate DNS queries
- Check cache for cached responses
- Process through filtering pipeline
- Forward to upstream if needed
- Log queries asynchronously

#### Server

DNS server managing UDP and TCP listeners:

```go
type Server struct {
    udpListener net.PacketConn
    tcpListener net.Listener
    handler     dns.Handler
    logger      *logging.Logger
}
```

### Key Interfaces

```go
// dns.Handler interface (from miekg/dns library)
type Handler interface {
    ServeDNS(ctx context.Context, w ResponseWriter, r *Msg)
}
```

### Component Interactions

**Inbound:**
- Receives queries from DNS clients (port 53)
- Receives configuration from Config package

**Outbound:**
- Queries Cache for cached responses
- Queries LocalRecords for local DNS entries
- Evaluates PolicyEngine rules
- Checks BlocklistManager for blocked domains
- Forwards to Forwarder for upstream resolution
- Logs to Storage asynchronously
- Reports metrics to Telemetry

### Thread Safety

**Concurrency Pattern:**
- Each DNS query handled in separate goroutine
- Single `RWMutex` for legacy map lookups (consolidated optimization)
- Lock-free reads from BlocklistManager using `atomic.Pointer`
- Read-heavy workload optimized with RWLock

**Race Conditions Prevented:**
- All map accesses protected by mutex
- Atomic operations for blocklist pointer
- Message pooling for reduced allocations

### Data Flow

```
Client Query → UDP/TCP Listener → Handler
    ↓
Cache Check (fast path)
    ↓ (miss)
Local Records Check
    ↓ (miss)
Policy Engine Evaluation
    ↓
Blocklist Check (lock-free)
    ↓ (not blocked)
Upstream Forward
    ↓
Cache Response
    ↓
Return to Client
```

### Performance Characteristics

- **Throughput:** 10,000+ queries/second (single core)
- **Latency:**
  - Cache hit: <5ms
  - Upstream forward: <50ms (depends on upstream)
  - Blocked query: <1ms
- **Memory:** ~100 bytes per in-flight query
- **Concurrency:** Unlimited concurrent queries (goroutines)

### Configuration Options

```yaml
server:
  listen_address: ":53"      # DNS server listen address
  tcp_enabled: true          # Enable TCP listener
  udp_enabled: true          # Enable UDP listener
  web_ui_address: ":8080"    # Web UI/API address
```

### Testing Approach

**Unit Tests:**
- Test each handler method in isolation
- Mock dependencies (cache, forwarder, etc.)
- Test error paths and edge cases

**Integration Tests:**
- End-to-end DNS query processing
- Test with real UDP/TCP connections
- Verify correct response formats

**Benchmark Tests:**
- Measure query processing latency
- Test throughput under load
- Profile memory allocations

**Files:**
- `pkg/dns/handler_test.go` - Handler unit tests
- `pkg/dns/handler_unit_test.go` - Isolated unit tests
- `pkg/dns/e2e_test.go` - End-to-end tests
- `pkg/dns/benchmark_test.go` - Performance benchmarks

---

## Blocklist Package

**Location:** `/home/erfi/gloryhole/pkg/blocklist`

### Purpose

Manages domain blocklists with automatic updates and lock-free lookups for maximum performance.

### Key Types

#### Manager

```go
type Manager struct {
    cfg        *config.Config
    downloader *Downloader
    logger     *logging.Logger

    // Current blocklist (atomic pointer for zero-copy reads)
    current atomic.Pointer[map[string]struct{}]

    // Lifecycle management
    updateTicker *time.Ticker
    stopChan     chan struct{}
    wg           sync.WaitGroup
    started      atomic.Bool
}
```

**Responsibilities:**
- Download blocklists from multiple sources
- Parse various formats (hosts, domains, adblock)
- Deduplicate domains across sources
- Automatic periodic updates
- Lock-free atomic updates

#### Downloader

```go
type Downloader struct {
    client *http.Client
    logger *logging.Logger
}
```

**Responsibilities:**
- HTTP download with retries
- Parse different blocklist formats
- Combine and deduplicate entries

### Key Methods

```go
// IsBlocked checks if domain is blocked (lock-free)
func (m *Manager) IsBlocked(domain string) bool

// Update downloads and updates blocklist
func (m *Manager) Update(ctx context.Context) error

// Start begins automatic update loop
func (m *Manager) Start(ctx context.Context) error

// Stop gracefully stops manager
func (m *Manager) Stop()
```

### Thread Safety

**Lock-Free Design:**
- Uses `atomic.Pointer` for current blocklist
- Zero-copy reads during lookups (no locks!)
- Atomic swap for updates
- No lock contention on hot path

**Performance:**
- Blocklist lookup: **~8ns** (atomic pointer read + map lookup)
- Update: ~1-5 seconds (depends on list size)
- Memory: ~50 bytes per blocked domain

### Component Interactions

**Inbound:**
- Receives configuration from Config
- Called by DNS Handler for lookups

**Outbound:**
- Downloads from remote URLs
- Logs to Logging package
- Reports metrics to Telemetry

### Data Flow

```
Start → Initial Download → Parse → Deduplicate → Atomic Update
                                                       ↓
                                                    Current ←─────┐
                                                       ↓           │
                                                    Lookups     Update
                                                                  Timer
```

### Performance Characteristics

- **Lookup:** ~8ns per query (lock-free)
- **Update:** 1-5 seconds for 500K domains
- **Memory:** ~50MB for 500K domains
- **Throughput:** 372M queries/second (benchmark)

### Configuration Options

```yaml
auto_update_blocklists: true
update_interval: "24h"
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"
  - "https://big.oisd.nl/domainswild"

whitelist:
  - "whitelisted-domain.com"
```

### Testing Approach

**Unit Tests:**
- Test blocklist loading and parsing
- Test domain matching logic
- Test atomic updates
- Test concurrent access

**Integration Tests:**
- Test auto-update loop
- Test multiple blocklist sources
- Test deduplication

**Benchmark Tests:**
- Measure lookup performance
- Test with various list sizes
- Benchmark update performance

**Files:**
- `pkg/blocklist/blocklist_test.go` - Blocklist parsing tests
- `pkg/blocklist/manager_test.go` - Manager tests

---

## Cache Package

**Location:** `/home/erfi/gloryhole/pkg/cache`

### Purpose

Provides thread-safe DNS response caching with LRU eviction and TTL support to reduce upstream queries.

### Key Types

#### Cache

```go
type Cache struct {
    cfg    *config.CacheConfig
    logger *logging.Logger

    // Single RWMutex for all cache operations
    mu sync.RWMutex

    // Cache entries indexed by cache key (domain + qtype)
    entries map[string]*cacheEntry

    // LRU tracking
    maxEntries int

    // Statistics
    stats cacheStats

    // Cleanup control
    stopCleanup chan struct{}
    cleanupDone chan struct{}
}
```

#### cacheEntry

```go
type cacheEntry struct {
    msg        *dns.Msg     // Cached DNS response (deep copy)
    expiresAt  time.Time    // When entry expires (based on TTL)
    lastAccess time.Time    // When last accessed (for LRU)
    size       int          // Size in bytes
}
```

### Key Methods

```go
// Get retrieves cached response (nil if not found/expired)
func (c *Cache) Get(ctx context.Context, r *dns.Msg) *dns.Msg

// Set stores DNS response with appropriate TTL
func (c *Cache) Set(ctx context.Context, r *dns.Msg, resp *dns.Msg)

// Clear removes all entries
func (c *Cache) Clear()

// Stats returns performance statistics
func (c *Cache) Stats() Stats
```

### Thread Safety

**Concurrency Pattern:**
- Single `RWMutex` for all operations
- Read lock for Get operations
- Write lock for Set, eviction, cleanup
- Background cleanup goroutine

**Safe Practices:**
- Deep copy of DNS messages to prevent mutations
- Proper lock ordering to prevent deadlocks
- Atomic statistics updates

### Component Interactions

**Inbound:**
- Called by DNS Handler for cache lookups
- Configuration from Config package

**Outbound:**
- Logs cache operations
- Reports hit/miss metrics to Telemetry

### Data Flow

```
Query → Cache Key Generation
           ↓
    Map Lookup (read lock)
           ↓
    ┌──────┴──────┐
    │             │
  Found       Not Found
    │             │
    ├─ Check TTL  └─► Return nil
    │
    ├─ Valid ──────► Update LRU → Return copy
    │
    └─ Expired ────► Delete → Return nil


Store → Determine TTL
           ↓
    Check max entries (write lock)
           ↓
    ┌──────┴──────┐
    │             │
  Full         Not Full
    │             │
    └─ Evict LRU  │
           │      │
           └──────┴─► Store entry
```

### Performance Characteristics

- **Get (hit):** ~100ns (read lock + map lookup + copy)
- **Get (miss):** ~50ns (read lock + map lookup)
- **Set:** ~200ns (write lock + map insert + eviction check)
- **Memory:** ~500 bytes per entry (including DNS message)
- **Hit Rate:** 50-70% typical (depends on query patterns)
- **Performance Boost:** 63% faster on cache hits

### Configuration Options

```yaml
cache:
  enabled: true
  max_entries: 10000         # Maximum cached responses
  min_ttl: "60s"             # Minimum TTL
  max_ttl: "24h"             # Maximum TTL
  negative_ttl: "5m"         # TTL for negative responses
```

### Testing Approach

**Unit Tests:**
- Test cache hit/miss
- Test TTL expiration
- Test LRU eviction
- Test concurrent access

**Integration Tests:**
- Test with DNS handler
- Test cleanup goroutine

**Benchmark Tests:**
- Measure get/set performance
- Test with various cache sizes
- Benchmark concurrent access

**Files:**
- `pkg/cache/cache_test.go` - Cache tests

---

## Forwarder Package

**Location:** `/home/erfi/gloryhole/pkg/forwarder`

### Purpose

Forwards DNS queries to upstream DNS servers with retry logic and connection pooling.

### Key Types

#### Forwarder

```go
type Forwarder struct {
    upstreams []string
    index     atomic.Uint32
    timeout   time.Duration
    retries   int
    logger    *logging.Logger

    // Connection pool
    clientPool sync.Pool
}
```

**Responsibilities:**
- Forward queries to upstream servers
- Round-robin load balancing
- Automatic retry on failure
- Connection pooling for performance
- Support both UDP and TCP

### Key Methods

```go
// Forward sends query to upstream via UDP
func (f *Forwarder) Forward(ctx context.Context, r *dns.Msg) (*dns.Msg, error)

// ForwardTCP sends query to upstream via TCP
func (f *Forwarder) ForwardTCP(ctx context.Context, r *dns.Msg) (*dns.Msg, error)

// Upstreams returns list of configured upstream servers
func (f *Forwarder) Upstreams() []string
```

### Thread Safety

**Concurrency Pattern:**
- `sync.Pool` for DNS client reuse
- `atomic.Uint32` for round-robin index
- No shared mutable state
- Each forward operation independent

### Component Interactions

**Inbound:**
- Called by DNS Handler when upstream resolution needed
- Configuration from Config package

**Outbound:**
- Connects to upstream DNS servers
- Logs queries and failures

### Data Flow

```
Query → Select Upstream (round-robin)
           ↓
    Get Client from Pool
           ↓
    Forward to Upstream
           ↓
    ┌──────┴──────┐
    │             │
  Success      Failure
    │             │
    └─► Return    └─► Retry with next upstream
        Response           ↓
                      ┌────┴────┐
                      │         │
                  Success   All Failed
                      │         │
                      └─► Return└─► Error
```

### Performance Characteristics

- **Latency:** 5-50ms (depends on upstream RTT)
- **Retries:** Up to 2 attempts per query
- **Timeout:** 2 seconds per attempt
- **Connection Pool:** Reuses UDP/TCP connections
- **Concurrency:** Unlimited parallel forwards

### Configuration Options

```yaml
upstream_dns_servers:
  - "1.1.1.1:53"      # Cloudflare
  - "8.8.8.8:53"      # Google
  - "9.9.9.9:53"      # Quad9
```

### Testing Approach

**Unit Tests:**
- Test round-robin selection
- Test retry logic
- Test timeout handling
- Mock upstream responses

**Integration Tests:**
- Test with real DNS servers
- Test fallback on failure

**Benchmark Tests:**
- Measure forwarding latency
- Test connection pool efficiency

**Files:**
- `pkg/forwarder/forwarder_test.go` - Forwarder tests

---

## Storage Package

**Location:** `/home/erfi/gloryhole/pkg/storage`

### Purpose

Provides multi-backend storage abstraction for query logging and statistics, supporting SQLite and Cloudflare D1.

### Key Types

#### Storage Interface

```go
type Storage interface {
    // Query Logging
    LogQuery(ctx context.Context, query *QueryLog) error
    GetRecentQueries(ctx context.Context, limit int) ([]*QueryLog, error)
    GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*QueryLog, error)
    GetQueriesByClientIP(ctx context.Context, clientIP string, limit int) ([]*QueryLog, error)

    // Statistics
    GetStatistics(ctx context.Context, since time.Time) (*Statistics, error)
    GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*DomainStats, error)
    GetBlockedCount(ctx context.Context, since time.Time) (int64, error)
    GetQueryCount(ctx context.Context, since time.Time) (int64, error)

    // Maintenance
    Cleanup(ctx context.Context, olderThan time.Time) error
    Close() error
    Ping(ctx context.Context) error
}
```

#### QueryLog

```go
type QueryLog struct {
    ID             int64
    Timestamp      time.Time
    ClientIP       string
    Domain         string
    QueryType      string    // A, AAAA, CNAME, etc.
    ResponseCode   int       // DNS response code
    Blocked        bool
    Cached         bool
    ResponseTimeMs int64
    Upstream       string
}
```

#### SQLiteStorage

```go
type SQLiteStorage struct {
    db     *sql.DB
    logger *logging.Logger

    // Buffered writes
    buffer     chan *QueryLog
    bufferSize int
    batchSize  int
    flushTimer *time.Timer
    stopChan   chan struct{}
    wg         sync.WaitGroup
}
```

### Key Methods

```go
// LogQuery asynchronously logs a DNS query
func (s *SQLiteStorage) LogQuery(ctx context.Context, query *QueryLog) error

// GetStatistics returns aggregated statistics
func (s *SQLiteStorage) GetStatistics(ctx context.Context, since time.Time) (*Statistics, error)

// GetTopDomains returns most queried domains
func (s *SQLiteStorage) GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*DomainStats, error)

// Cleanup removes old logs based on retention policy
func (s *SQLiteStorage) Cleanup(ctx context.Context, olderThan time.Time) error
```

### Thread Safety

**Concurrency Pattern:**
- Buffered channel for async writes
- Single writer goroutine for batch inserts
- Read queries use database connection pool
- WAL mode for concurrent reads/writes

**Async Logging:**
```
DNS Query → Buffer (channel) → Batch Writer → SQLite
              (1000 entries)    (every 5s)     (WAL mode)
                                                    ↓
                                            <10μs overhead
```

### Component Interactions

**Inbound:**
- Called by DNS Handler for query logging
- Called by API Server for statistics

**Outbound:**
- Writes to SQLite database
- Logs errors to Logging package

### Data Flow

```
LogQuery → Buffer Channel
              ↓
    ┌─────────┴─────────┐
    │                   │
Buffer Full       Timer Fires
    │                   │
    └─────────┬─────────┘
              ↓
    Batch Insert (transaction)
              ↓
         Commit
```

### Performance Characteristics

- **Write Latency:** <10μs (buffered, async)
- **Batch Size:** 100-1000 queries per transaction
- **Flush Interval:** 5 seconds
- **Read Latency:** 1-10ms (depends on query complexity)
- **Storage:** ~200 bytes per query log
- **Retention:** Configurable (default: 7 days)

### Database Schema

```sql
CREATE TABLE queries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    client_ip TEXT NOT NULL,
    domain TEXT NOT NULL,
    query_type TEXT NOT NULL,
    response_code INTEGER NOT NULL,
    blocked INTEGER NOT NULL,
    cached INTEGER NOT NULL,
    response_time_ms INTEGER NOT NULL,
    upstream TEXT
);

CREATE INDEX idx_queries_timestamp ON queries(timestamp);
CREATE INDEX idx_queries_domain ON queries(domain);
CREATE INDEX idx_queries_client_ip ON queries(client_ip);
CREATE INDEX idx_queries_blocked ON queries(blocked);
```

### Configuration Options

```yaml
database:
  enabled: true
  backend: "sqlite"
  sqlite:
    path: "./glory-hole.db"
    wal_mode: true
    busy_timeout: 5000
    cache_size: 10000
  buffer_size: 1000
  flush_interval: "5s"
  retention_days: 7
```

### Testing Approach

**Unit Tests:**
- Test async buffering
- Test batch inserts
- Test statistics queries
- Test cleanup

**Integration Tests:**
- Test with real SQLite database
- Test concurrent reads/writes
- Test retention policies

**Files:**
- `pkg/storage/storage_test.go` - Interface tests
- `pkg/storage/sqlite_test.go` - SQLite implementation tests

---

## Policy Package

**Location:** `/home/erfi/gloryhole/pkg/policy`

### Purpose

Advanced rule-based filtering engine using expression language for complex DNS filtering policies.

### Key Types

#### Engine

```go
type Engine struct {
    rules []*Rule
    mu    sync.RWMutex
}
```

#### Rule

```go
type Rule struct {
    Name       string        // Human-readable name
    Logic      string        // Expression to evaluate
    Action     string        // BLOCK, ALLOW, or REDIRECT
    ActionData string        // Optional action data
    Enabled    bool          // Whether rule is active
    program    *vm.Program   // Compiled expression
}
```

#### Context

```go
type Context struct {
    Domain    string        // FQDN being queried
    ClientIP  string        // Client IP address
    QueryType string        // A, AAAA, CNAME, etc.
    Hour      int           // 0-23
    Minute    int           // 0-59
    Day       int           // 1-31
    Month     int           // 1-12
    Weekday   int           // 0-6 (Sunday=0)
    Time      time.Time     // Full timestamp
}
```

### Key Methods

```go
// AddRule compiles and adds a rule
func (e *Engine) AddRule(rule *Rule) error

// Evaluate checks all rules against context
func (e *Engine) Evaluate(ctx Context) (bool, *Rule)

// RemoveRule removes rule by name
func (e *Engine) RemoveRule(name string) bool

// GetRules returns all rules
func (e *Engine) GetRules() []*Rule
```

### Helper Functions

Available in policy expressions:

**Domain Functions:**
- `DomainMatches(domain, pattern)` - Contains match
- `DomainEndsWith(domain, suffix)` - Suffix match
- `DomainStartsWith(domain, prefix)` - Prefix match
- `DomainRegex(domain, pattern)` - Regex match
- `DomainLevelCount(domain)` - Count domain levels

**IP Functions:**
- `IPInCIDR(ip, cidr)` - Check if IP in CIDR range
- `IPEquals(ip1, ip2)` - Compare IPs

**Time Functions:**
- `IsWeekend(weekday)` - Check if Saturday/Sunday
- `InTimeRange(h, m, startH, startM, endH, endM)` - Time range check

**Query Functions:**
- `QueryTypeIn(type, ...types)` - Check query type

### Thread Safety

**Concurrency Pattern:**
- `RWMutex` protects rule list
- Pre-compiled programs (read-only after compilation)
- Multiple concurrent evaluations safe

### Component Interactions

**Inbound:**
- Called by DNS Handler for policy evaluation
- Configured via Config or API

**Outbound:**
- None (self-contained evaluation)

### Data Flow

```
Query Context → For each enabled rule
                       ↓
                Run compiled program
                       ↓
                 ┌─────┴─────┐
                 │           │
              Match       No Match
                 │           │
           Return Action  Continue
                             ↓
                      All rules checked
                             ↓
                        No match
```

### Performance Characteristics

- **Compilation:** ~1ms per rule (startup only)
- **Evaluation:** <1μs per rule
- **Memory:** ~1KB per compiled rule
- **Concurrency:** Unlimited parallel evaluations

### Configuration Options

```yaml
policy:
  enabled: true
  rules:
    - name: "Block Social Media After Hours"
      logic: "(Hour >= 22 || Hour < 6) && DomainMatches(Domain, 'facebook')"
      action: "BLOCK"
      enabled: true

    - name: "Allow Work Domains"
      logic: "Hour >= 9 && Hour <= 17 && DomainEndsWith(Domain, '.company.com')"
      action: "ALLOW"
      enabled: true

    - name: "Block Kids on Weekends"
      logic: "IsWeekend(Weekday) && IPInCIDR(ClientIP, '192.168.1.0/24')"
      action: "BLOCK"
      enabled: true
```

### Testing Approach

**Unit Tests:**
- Test each helper function
- Test rule compilation
- Test evaluation logic
- Test concurrent access

**Integration Tests:**
- Test with DNS handler
- Test complex expressions
- Test performance

**Benchmark Tests:**
- Measure evaluation performance
- Test with many rules

**Files:**
- `pkg/policy/engine_test.go` - Engine tests
- `pkg/policy/helpers_test.go` - Helper function tests
- `pkg/policy/benchmark_test.go` - Performance benchmarks

---

## API Package

**Location:** `/home/erfi/gloryhole/pkg/api`

### Purpose

Provides REST API and Web UI for monitoring, management, and configuration.

### Key Types

#### Server

```go
type Server struct {
    handler    http.Handler
    httpServer *http.Server
    logger     *slog.Logger

    // Dependencies
    storage          storage.Storage
    blocklistManager *blocklist.Manager
    policyEngine     *policy.Engine

    // Metadata
    version   string
    startTime time.Time
}
```

### Key Endpoints

**Health & Monitoring:**
- `GET /api/health` - Health check with uptime
- `GET /healthz` - Kubernetes liveness probe
- `GET /readyz` - Kubernetes readiness probe
- `GET /api/stats` - Query statistics
- `GET /api/queries` - Recent queries
- `GET /api/top-domains` - Most queried domains

**Management:**
- `POST /api/blocklist/reload` - Reload blocklists

**Policy Management:**
- `GET /api/policies` - List all policies
- `POST /api/policies` - Create policy
- `GET /api/policies/{id}` - Get policy
- `PUT /api/policies/{id}` - Update policy
- `DELETE /api/policies/{id}` - Delete policy

**Web UI:**
- `GET /` - Dashboard
- `GET /queries` - Query log viewer
- `GET /policies` - Policy management
- `GET /settings` - Settings page

### Thread Safety

**Concurrency Pattern:**
- HTTP server handles concurrent requests
- Read-only access to dependencies
- Thread-safe dependency interfaces

### Component Interactions

**Inbound:**
- HTTP requests from users/monitoring tools

**Outbound:**
- Queries Storage for statistics
- Calls BlocklistManager for reloads
- Calls PolicyEngine for management

### Data Flow

```
HTTP Request → Router → Middleware Chain → Handler
                            ↓                  ↓
                    Logging, CORS         Query Storage
                                               ↓
                                          Format Response
                                               ↓
                                          HTTP Response
```

### Performance Characteristics

- **Latency:** 1-10ms per request
- **Throughput:** 1000+ requests/second
- **Timeout:** 10 seconds per request
- **Concurrent Connections:** Unlimited

### Configuration Options

```yaml
server:
  web_ui_address: ":8080"
```

### Testing Approach

**Unit Tests:**
- Test each endpoint handler
- Test middleware
- Test error handling
- Mock dependencies

**Integration Tests:**
- Test with httptest server
- Test full request/response cycle

**Files:**
- `pkg/api/api_test.go` - API tests
- `pkg/api/api_additional_test.go` - Additional tests
- `pkg/api/handlers_policy_test.go` - Policy handler tests
- `pkg/api/ui_handlers_test.go` - UI handler tests

---

## Config Package

**Location:** `/home/erfi/gloryhole/pkg/config`

### Purpose

Manages application configuration with YAML support and hot-reload capability.

### Key Types

#### Config

```go
type Config struct {
    Server               ServerConfig
    UpstreamDNSServers   []string
    UpdateInterval       time.Duration
    AutoUpdateBlocklists bool
    Blocklists           []string
    Whitelist            []string
    Overrides            map[string]string
    CNAMEOverrides       map[string]string
    Database             storage.Config
    Cache                CacheConfig
    LocalRecords         LocalRecordsConfig
    Policy               PolicyConfig
    Logging              LoggingConfig
    Telemetry            TelemetryConfig
}
```

#### Watcher

```go
type Watcher struct {
    cfg      *Config
    path     string
    watcher  *fsnotify.Watcher
    onChange func(*Config)
    logger   *logging.Logger
}
```

### Key Methods

```go
// Load loads configuration from YAML file
func Load(path string) (*Config, error)

// Watch starts watching config file for changes
func (w *Watcher) Watch() error

// Stop stops the watcher
func (w *Watcher) Stop() error
```

### Thread Safety

**Concurrency Pattern:**
- Immutable config objects (no mutations)
- New config instance on reload
- Callback notification for changes

### Component Interactions

**Inbound:**
- Reads YAML configuration file

**Outbound:**
- Provides config to all components
- Notifies on config changes

### Configuration Options

See example in `/home/erfi/gloryhole/config.example.yml`

### Testing Approach

**Unit Tests:**
- Test YAML parsing
- Test validation
- Test file watching
- Test hot-reload

**Files:**
- `pkg/config/config_test.go` - Config tests
- `pkg/config/watcher_test.go` - Watcher tests

---

## Logging Package

**Location:** `/home/erfi/gloryhole/pkg/logging`

### Purpose

Provides structured logging using Go's standard `log/slog` package.

### Key Types

#### Logger

Wrapper around `slog.Logger` with convenience methods:

```go
type Logger struct {
    *slog.Logger
}
```

### Key Methods

```go
func NewLogger(cfg *Config) *Logger
func (l *Logger) Debug(msg string, args ...any)
func (l *Logger) Info(msg string, args ...any)
func (l *Logger) Warn(msg string, args ...any)
func (l *Logger) Error(msg string, args ...any)
```

### Configuration Options

```yaml
logging:
  level: "info"              # debug, info, warn, error
  format: "json"             # json, text
  output: "stdout"           # stdout, stderr, file
  file_path: "/var/log/glory-hole.log"
  add_source: false          # Include source file/line
```

### Testing Approach

**Unit Tests:**
- Test log levels
- Test formatting
- Test file output

**Files:**
- `pkg/logging/logger_test.go` - Logger tests

---

## Telemetry Package

**Location:** `/home/erfi/gloryhole/pkg/telemetry`

### Purpose

Provides OpenTelemetry metrics and Prometheus exporter for monitoring.

### Key Metrics

- `dns_queries_total` - Total DNS queries
- `dns_queries_blocked` - Blocked queries
- `dns_queries_cached` - Cached queries
- `dns_query_duration` - Query latency histogram
- `cache_hit_rate` - Cache hit rate
- `blocklist_size` - Number of blocked domains

### Configuration Options

```yaml
telemetry:
  enabled: true
  prometheus_enabled: true
  prometheus_port: 9090
  tracing_enabled: false
```

### Testing Approach

**Files:**
- `pkg/telemetry/telemetry_test.go` - Telemetry tests

---

## Local Records Package

**Location:** `/home/erfi/gloryhole/pkg/localrecords`

### Purpose

Manages local DNS records (A/AAAA/CNAME) with wildcard support for custom domain resolution.

### Key Types

#### Manager

```go
type Manager struct {
    records      map[string]*Record
    wildcards    map[string]*Record
    cnameTargets map[string]string
    mu           sync.RWMutex
}
```

### Key Methods

```go
func (m *Manager) LookupA(domain string) ([]net.IP, uint32, bool)
func (m *Manager) LookupAAAA(domain string) ([]net.IP, uint32, bool)
func (m *Manager) LookupCNAME(domain string) (string, uint32, bool)
func (m *Manager) ResolveCNAME(domain string, maxDepth int) ([]net.IP, uint32, bool)
```

### Features

- A/AAAA record support
- CNAME with automatic chain resolution
- Wildcard domains (*.local)
- Multiple IPs per record
- Custom TTL per record

### Testing Approach

**Files:**
- `pkg/localrecords/records_test.go` - Record tests
- `pkg/localrecords/util_test.go` - Utility tests

---

## Component Interactions

### Complete Request Flow

```
1. Client sends DNS query
        ↓
2. DNS Server receives (UDP/TCP)
        ↓
3. DNS Handler processes
        ↓
4. Check Cache (Cache pkg)
        ↓ (miss)
5. Check Local Records (LocalRecords pkg)
        ↓ (miss)
6. Evaluate Policy (Policy pkg)
        ↓
7. Check Blocklist (Blocklist pkg)
        ↓ (not blocked)
8. Forward upstream (Forwarder pkg)
        ↓
9. Cache response (Cache pkg)
        ↓
10. Log query (Storage pkg, async)
        ↓
11. Update metrics (Telemetry pkg)
        ↓
12. Return response to client
```

### Dependency Graph

```
            Config
              ↓
    ┌─────────┼─────────┐
    ↓         ↓         ↓
Logging  Telemetry  Storage
    ↓         ↓         ↓
    └────┬────┴────┬────┘
         ↓         ↓
    Components   API
         ↓         ↓
    DNS Handler ←─┘
         ↓
    DNS Server
```

---

## Summary

Glory-Hole's modular architecture provides:

- **Clean separation of concerns** - Each package has single responsibility
- **Thread-safe by design** - Proper locking and atomic operations
- **High performance** - Lock-free hot paths, connection pooling, caching
- **Testable** - Interfaces, dependency injection, comprehensive tests
- **Maintainable** - Clear APIs, good documentation, consistent patterns

**Performance Highlights:**
- 10,000+ queries/second throughput
- <5ms cache hit latency
- <1ms blocked query latency
- 8ns blocklist lookup (lock-free)
- 82.5% test coverage

For more details, see:
- [Architecture Overview](overview.md)
- [Performance Documentation](performance.md)
- [Testing Documentation](testing.md)
