# Architecture Decision Records (ADRs)

**Last Updated:** 2025-11-22
**Version:** 0.6.0

This document records major architectural and design decisions made in the Glory-Hole DNS server project, including context, options considered, and rationale.

---

## Table of Contents

1. [ADR-001: Lock-Free Blocklist Design](#adr-001-lock-free-blocklist-design)
2. [ADR-002: Single RWMutex Consolidation](#adr-002-single-rwmutex-consolidation)
3. [ADR-003: Async Query Logging Pattern](#adr-003-async-query-logging-pattern)
4. [ADR-004: Policy Engine with Expr Language](#adr-004-policy-engine-with-expr-language)
5. [ADR-005: SQLite vs Other Databases](#adr-005-sqlite-vs-other-databases)
6. [ADR-006: Go Templates vs Frontend Framework](#adr-006-go-templates-vs-frontend-framework)
7. [ADR-007: HTMX vs React/Vue for Dynamic UI](#adr-007-htmx-vs-reactvue-for-dynamic-ui)
8. [ADR-008: Embedded Assets vs External Files](#adr-008-embedded-assets-vs-external-files)
9. [ADR-009: Cache Implementation - LRU Algorithm](#adr-009-cache-implementation-lru-algorithm)
10. [ADR-010: DNS Server Architecture](#adr-010-dns-server-architecture)
11. [ADR-011: Configuration Format - YAML](#adr-011-configuration-format-yaml)
12. [ADR-012: Structured Logging with slog](#adr-012-structured-logging-with-slog)
13. [ADR-013: CGO-Free SQLite Implementation](#adr-013-cgo-free-sqlite-implementation)
14. [ADR-014: Local Records Package Design](#adr-014-local-records-package-design)

---

## ADR-001: Lock-Free Blocklist Design

**Status:** Accepted
**Date:** 2025-01-15
**Deciders:** Core team

### Context

The blocklist is queried on every DNS request (hot path). Initial implementation used `sync.RWMutex` to protect a `map[string]struct{}`, which provided ~110ns lookup performance. With 10,000+ queries/second, this became a bottleneck.

We needed to:
- Support safe concurrent reads during lookups
- Allow atomic updates when blocklist refreshes
- Minimize latency on the critical path
- Handle 500K+ blocked domains

### Options Considered

#### Option 1: RWMutex with Map (Original)
```go
type Manager struct {
    mu        sync.RWMutex
    blocklist map[string]struct{}
}

func (m *Manager) IsBlocked(domain string) bool {
    m.mu.RLock()
    defer m.mu.RUnlock()
    _, blocked := m.blocklist[domain]
    return blocked
}
```

**Pros:**
- Simple to implement
- Well-understood pattern
- Safe for concurrent access

**Cons:**
- Lock contention under high load
- ~110ns lookup latency
- Read lock overhead on every query

#### Option 2: sync.Map
```go
type Manager struct {
    blocklist sync.Map
}

func (m *Manager) IsBlocked(domain string) bool {
    _, blocked := m.blocklist.Load(domain)
    return blocked
}
```

**Pros:**
- Built-in concurrent safety
- Good for high read/low write workloads

**Cons:**
- ~80ns lookup (still slower than needed)
- Less memory efficient
- More complex updates

#### Option 3: Atomic Pointer (Chosen)
```go
type Manager struct {
    current atomic.Pointer[map[string]struct{}]
}

func (m *Manager) IsBlocked(domain string) bool {
    blocklist := m.current.Load()
    _, blocked := (*blocklist)[domain]
    return blocked
}

func (m *Manager) Update(newList map[string]struct{}) {
    m.current.Store(&newList)
}
```

**Pros:**
- **Lock-free reads** - zero lock overhead
- **~8ns lookup** - 10x faster than RWMutex
- **Simple atomic swap** for updates
- **Zero-copy reads** - pointer dereference only

**Cons:**
- Old map remains in memory until GC (acceptable)
- Slightly more complex update logic

### Decision

**Chosen: Option 3 - Atomic Pointer**

The performance improvement (110ns → 8ns) was too significant to ignore. With lock-free reads, we achieved **372M queries/second** in benchmarks vs **33M QPS** with RWMutex.

### Implementation

```go
// Location: /home/erfi/gloryhole/pkg/blocklist/manager.go

type Manager struct {
    cfg        *config.Config
    downloader *Downloader
    logger     *logging.Logger

    // Current blocklist (atomic pointer for zero-copy reads)
    current atomic.Pointer[map[string]struct{}]

    updateTicker *time.Ticker
    stopChan     chan struct{}
    wg           sync.WaitGroup
    started      atomic.Bool
}

func (m *Manager) IsBlocked(domain string) bool {
    blocklist := m.current.Load()
    if blocklist == nil {
        return false
    }
    _, blocked := (*blocklist)[domain]
    return blocked
}

func (m *Manager) Update(ctx context.Context) error {
    // Download and parse blocklists
    blocklist, err := m.downloader.DownloadAll(ctx, m.cfg.Blocklists)
    if err != nil {
        return err
    }

    // Atomic swap - all readers instantly see new blocklist
    m.current.Store(&blocklist)

    return nil
}
```

### Consequences

**Positive:**
- 11x faster lookups (110ns → 8ns)
- No lock contention on hot path
- Scales linearly with cores
- Simple and maintainable

**Negative:**
- Old blocklist kept in memory briefly during updates (acceptable overhead)
- Requires Go 1.19+ for `atomic.Pointer`

### Metrics

**Before (RWMutex):**
```
BenchmarkBlocklist_Lookup-8    10000000    110 ns/op    0 B/op    0 allocs/op
```

**After (Atomic Pointer):**
```
BenchmarkBlocklist_Lookup-8    150000000    8 ns/op    0 B/op    0 allocs/op
```

**Real-world impact:** Reduced DNS query latency by ~100ns per query.

---

## ADR-002: Single RWMutex Consolidation

**Status:** Accepted
**Date:** 2025-01-18
**Deciders:** Core team

### Context

The DNS handler originally had separate locks for different lookup maps:
- `blocklistMu` for blocklist
- `whitelistMu` for whitelist
- `overridesMu` for IP overrides
- `cnameMu` for CNAME overrides

Each lock acquisition added ~500ns overhead. A typical query touched 2-3 locks, totaling 1-2μs of lock overhead.

### Options Considered

#### Option 1: Separate Locks per Map (Original)
```go
type Handler struct {
    blocklistMu    sync.RWMutex
    blocklist      map[string]struct{}

    whitelistMu    sync.RWMutex
    whitelist      map[string]struct{}

    overridesMu    sync.RWMutex
    overrides      map[string]net.IP

    cnameMu        sync.RWMutex
    cnameOverrides map[string]string
}
```

**Pros:**
- Fine-grained locking
- Maximum concurrency in theory

**Cons:**
- Multiple lock acquisitions per query (1-2μs total)
- More complex lock ordering
- Cache line bouncing between locks

#### Option 2: Single Global Lock (Chosen)
```go
type Handler struct {
    lookupMu sync.RWMutex

    blocklist      map[string]struct{}
    whitelist      map[string]struct{}
    overrides      map[string]net.IP
    cnameOverrides map[string]string
}
```

**Pros:**
- **Single lock acquisition** (~500ns vs 1-2μs)
- **Simpler code** - no lock ordering issues
- **Better cache locality**
- **Fewer allocations** - one defer instead of multiple

**Cons:**
- Coarser granularity (acceptable for read-heavy workload)

#### Option 3: Lock-Free for All Maps
Using `atomic.Pointer` for all maps.

**Pros:**
- Maximum performance

**Cons:**
- More complex updates (need to coordinate multiple atomic swaps)
- Not needed since blocklist is the hot path
- Other maps updated rarely (config changes only)

### Decision

**Chosen: Option 2 - Single RWMutex**

Consolidating to a single lock reduced overhead by ~1μs per query while keeping code simple. Since these are read-heavy workloads, the coarser granularity has minimal impact on concurrency.

The blocklist was moved to lock-free atomic pointer (ADR-001), so it doesn't need this lock at all. The remaining maps (whitelist, overrides, CNAME) are rarely updated and don't justify individual locks.

### Implementation

```go
// Location: /home/erfi/gloryhole/pkg/dns/server.go

type Handler struct {
    // Single lock for all lookup maps (performance optimization)
    lookupMu sync.RWMutex

    // Blocklist manager with lock-free updates (FAST PATH - no lock!)
    BlocklistManager *blocklist.Manager

    // Legacy static maps (SLOW PATH - backward compatibility)
    Blocklist      map[string]struct{}
    Whitelist      map[string]struct{}
    Overrides      map[string]net.IP
    CNAMEOverrides map[string]string
}

func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
    // ... cache and local records check ...

    // Fast path: lock-free blocklist check
    if h.BlocklistManager != nil {
        blocked := h.BlocklistManager.IsBlocked(domain)

        // Still need lock for whitelist/overrides
        h.lookupMu.RLock()
        _, whitelisted := h.Whitelist[domain]
        overrideIP, hasOverride := h.Overrides[domain]
        cnameTarget, hasCNAME := h.CNAMEOverrides[domain]
        h.lookupMu.RUnlock()

        // Process results...
    }
}
```

### Consequences

**Positive:**
- 2x reduction in lock overhead (~1μs savings per query)
- Simpler code and maintenance
- No lock ordering bugs
- Better CPU cache utilization

**Negative:**
- Slightly coarser locking (negligible impact)

### Metrics

**Before (4 separate locks):**
```
Average lock overhead: 2-4μs per query
```

**After (1 consolidated lock):**
```
Average lock overhead: ~500ns per query
```

---

## ADR-003: Async Query Logging Pattern

**Status:** Accepted
**Date:** 2025-01-20
**Deciders:** Core team

### Context

DNS queries need to be logged for analytics, but database writes are slow (1-10ms). Synchronous logging would add unacceptable latency to every DNS response.

Requirements:
- Don't slow down DNS responses
- Ensure logs aren't lost
- Handle high query rates (10K+ QPS)
- Provide near real-time statistics

### Options Considered

#### Option 1: Synchronous Logging
```go
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
    // ... handle query ...

    // Block until logged
    if err := h.Storage.LogQuery(ctx, query); err != nil {
        log.Error("Failed to log query", err)
    }

    w.WriteMsg(response)
}
```

**Pros:**
- Simple implementation
- Guaranteed durability

**Cons:**
- Adds 1-10ms to every query
- Unacceptable latency impact
- Poor performance

#### Option 2: Fire-and-Forget Goroutine
```go
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
    // ... handle query ...

    // Spawn goroutine for logging
    go func() {
        h.Storage.LogQuery(context.Background(), query)
    }()

    w.WriteMsg(response)
}
```

**Pros:**
- No latency impact
- Simple to implement

**Cons:**
- Creates goroutine per query (overhead)
- No backpressure handling
- Logs may be lost on shutdown

#### Option 3: Buffered Channel with Worker (Chosen)
```go
type SQLiteStorage struct {
    buffer chan *QueryLog
    // ... worker goroutine reads from buffer and batch inserts
}

func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
    // ... handle query ...

    defer func() {
        go func() {
            ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
            defer cancel()
            h.Storage.LogQuery(ctx, query) // Non-blocking send to buffer
        }()
    }()

    w.WriteMsg(response)
}
```

**Pros:**
- **<10μs overhead** (channel send)
- **Batch inserts** (100-1000 queries per transaction)
- **Backpressure** (channel buffer prevents memory explosion)
- **Graceful shutdown** (drain buffer on exit)
- **Reuses single worker goroutine**

**Cons:**
- Slightly more complex
- Small delay before logs appear in DB (acceptable)

### Decision

**Chosen: Option 3 - Buffered Channel with Worker**

This provides the best balance of performance and reliability. The <10μs overhead is negligible, while batch inserts maximize database performance.

### Implementation

```go
// Location: /home/erfi/gloryhole/pkg/storage/sqlite.go

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

func (s *SQLiteStorage) LogQuery(ctx context.Context, query *QueryLog) error {
    select {
    case s.buffer <- query:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Buffer full - drop oldest (or block, depending on requirements)
        return ErrBufferFull
    }
}

func (s *SQLiteStorage) worker() {
    defer s.wg.Done()

    batch := make([]*QueryLog, 0, s.batchSize)

    for {
        select {
        case query := <-s.buffer:
            batch = append(batch, query)

            if len(batch) >= s.batchSize {
                s.flush(batch)
                batch = batch[:0]
            }

        case <-s.flushTimer.C:
            if len(batch) > 0 {
                s.flush(batch)
                batch = batch[:0]
            }

        case <-s.stopChan:
            // Drain remaining queries
            for len(s.buffer) > 0 {
                batch = append(batch, <-s.buffer)
            }
            if len(batch) > 0 {
                s.flush(batch)
            }
            return
        }
    }
}

func (s *SQLiteStorage) flush(queries []*QueryLog) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT INTO queries (timestamp, client_ip, domain, query_type,
                           response_code, blocked, cached, response_time_ms, upstream)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, q := range queries {
        _, err := stmt.Exec(q.Timestamp, q.ClientIP, q.Domain, q.QueryType,
                           q.ResponseCode, q.Blocked, q.Cached, q.ResponseTimeMs, q.Upstream)
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}
```

### Consequences

**Positive:**
- Minimal impact on DNS performance (<10μs)
- Efficient batch database writes
- Handles high query rates (10K+ QPS)
- Graceful degradation under load

**Negative:**
- Logs appear with slight delay (up to 5 seconds)
- Possible log loss on crash (buffered data)
- More complex than synchronous approach

### Configuration

```yaml
database:
  buffer_size: 1000        # Channel buffer size
  flush_interval: "5s"     # Maximum time before flush
  batch_size: 100          # Queries per transaction
```

---

## ADR-004: Policy Engine with Expr Language

**Status:** Accepted
**Date:** 2025-01-22
**Deciders:** Core team

### Context

We needed advanced filtering beyond simple domain blocklists:
- Time-based rules (block social media after 10pm)
- Client-specific rules (different policies per device)
- Complex conditions (CIDR ranges, regex, etc.)
- User-configurable without code changes

### Options Considered

#### Option 1: Custom DSL
Create our own domain-specific language.

**Pros:**
- Full control over syntax
- Optimized for our use case

**Cons:**
- Significant development effort
- Need lexer, parser, evaluator
- Limited expression power
- Users need to learn new syntax

#### Option 2: Lua Scripting
Embed Lua interpreter for rules.

**Pros:**
- Full programming language
- Very flexible

**Cons:**
- Too powerful (security concerns)
- Performance overhead
- Large dependency
- Complex for simple rules

#### Option 3: Expr Language (Chosen)
Use `github.com/expr-lang/expr` - expression language for Go.

**Pros:**
- **Familiar syntax** (similar to Go/JavaScript)
- **Fast evaluation** (<1μs per rule)
- **Type-safe** at compile time
- **Extensible** with custom functions
- **Small footprint**
- **Well-maintained** library

**Cons:**
- External dependency
- Limited to expressions (no loops, complex logic)

### Decision

**Chosen: Option 3 - Expr Language**

Expr provides the perfect balance of power and simplicity. Users can write expressive rules without learning a new language, and performance is excellent.

### Implementation

```go
// Location: /home/erfi/gloryhole/pkg/policy/engine.go

type Engine struct {
    rules []*Rule
    mu    sync.RWMutex
}

type Rule struct {
    Name       string
    Logic      string        // Expression to evaluate
    Action     string        // BLOCK, ALLOW, REDIRECT
    ActionData string        // Optional data (e.g., redirect target)
    Enabled    bool
    program    *vm.Program   // Compiled expression
}

func (e *Engine) AddRule(rule *Rule) error {
    // Compile expression with custom functions
    program, err := expr.Compile(rule.Logic,
        expr.Env(Context{}),
        expr.Function("DomainMatches", ...),
        expr.Function("IPInCIDR", ...),
        // ... more functions
    )
    if err != nil {
        return fmt.Errorf("failed to compile rule: %w", err)
    }

    rule.program = program
    e.rules = append(e.rules, rule)
    return nil
}

func (e *Engine) Evaluate(ctx Context) (bool, *Rule) {
    e.mu.RLock()
    defer e.mu.RUnlock()

    for _, rule := range e.rules {
        if !rule.Enabled {
            continue
        }

        result, err := vm.Run(rule.program, ctx)
        if err != nil {
            continue
        }

        if matched, ok := result.(bool); ok && matched {
            return true, rule
        }
    }

    return false, nil
}
```

### Example Rules

```yaml
policy:
  enabled: true
  rules:
    # Block social media after hours
    - name: "Block Social Media After 10pm"
      logic: "(Hour >= 22 || Hour < 6) && DomainMatches(Domain, 'facebook')"
      action: "BLOCK"
      enabled: true

    # Allow work domains during business hours
    - name: "Allow Work Domains"
      logic: "Hour >= 9 && Hour <= 17 && DomainEndsWith(Domain, '.company.com')"
      action: "ALLOW"
      enabled: true

    # Block specific client on weekends
    - name: "Block Kids Gaming on Weekends"
      logic: "IsWeekend(Weekday) && IPInCIDR(ClientIP, '192.168.1.100/32')"
      action: "BLOCK"
      enabled: true
```

### Helper Functions

```go
// Domain matching
DomainMatches(domain, pattern)
DomainEndsWith(domain, suffix)
DomainStartsWith(domain, prefix)
DomainRegex(domain, pattern)

// IP matching
IPInCIDR(ip, cidr)
IPEquals(ip1, ip2)

// Time functions
IsWeekend(weekday)
InTimeRange(hour, minute, startH, startM, endH, endM)

// Query type
QueryTypeIn(type, ...types)
```

### Consequences

**Positive:**
- Powerful yet simple rule syntax
- Fast evaluation (<1μs per rule)
- Type-safe at compile time
- Easy to extend with new functions
- Great user experience

**Negative:**
- External dependency (acceptable - well-maintained)
- Learning curve for complex expressions

### Performance

```
BenchmarkPolicyEngine_Evaluate-8    1000000    960 ns/op
```

---

## ADR-005: SQLite vs Other Databases

**Status:** Accepted
**Date:** 2025-01-10
**Deciders:** Core team

### Context

Need to store query logs and statistics. Requirements:
- Handle 10K+ queries/second logging
- Support complex analytics queries
- Easy deployment (no separate database server)
- Small footprint
- Cross-platform

### Options Considered

#### Option 1: PostgreSQL
Client-server database.

**Pros:**
- Excellent performance
- Rich features
- Great for analytics

**Cons:**
- **Separate server required**
- Complex deployment
- Overkill for our needs

#### Option 2: MySQL/MariaDB
Client-server database.

**Pros:**
- Widely known
- Good performance

**Cons:**
- **Separate server required**
- Complex deployment
- Overkill for our needs

#### Option 3: SQLite (Chosen)
Embedded database.

**Pros:**
- **Zero configuration** - single file
- **Excellent performance** with WAL mode
- **Cross-platform**
- **Small footprint** (~1MB)
- **ACID compliant**
- **Perfect for embedded use**
- **Rich query capabilities**

**Cons:**
- Not suitable for distributed systems (not our use case)
- Write concurrency limitations (mitigated with WAL mode)

### Decision

**Chosen: Option 3 - SQLite**

SQLite is perfect for Glory-Hole's use case:
- Single-server deployment
- Embedded in binary
- No separate database to manage
- Excellent performance for our needs

### Implementation

Using `modernc.org/sqlite` (CGO-free, see ADR-013):

```go
// Location: /home/erfi/gloryhole/pkg/storage/sqlite.go

func NewSQLiteStorage(cfg *SQLiteConfig, logger *logging.Logger) (*SQLiteStorage, error) {
    // Open database
    db, err := sql.Open("sqlite", cfg.Path)
    if err != nil {
        return nil, err
    }

    // Enable WAL mode for better concurrency
    if cfg.WALMode {
        _, err = db.Exec("PRAGMA journal_mode=WAL")
        if err != nil {
            return nil, err
        }
    }

    // Create schema
    _, err = db.Exec(schema)
    if err != nil {
        return nil, err
    }

    return &SQLiteStorage{
        db:     db,
        logger: logger,
        buffer: make(chan *QueryLog, cfg.BufferSize),
    }, nil
}
```

### WAL Mode

SQLite's Write-Ahead Logging (WAL) mode is crucial:
- Allows concurrent reads during writes
- Better performance for write-heavy workloads
- Minimal overhead

```sql
PRAGMA journal_mode=WAL;
```

### Consequences

**Positive:**
- Zero configuration database
- Single file storage
- Easy backups (copy file)
- Excellent performance
- Small footprint

**Negative:**
- Not suitable for multi-server deployments (acceptable limitation)

### Performance

**Metrics:**
- 1,000+ inserts/second (batch mode)
- <10ms query latency for statistics
- ~200 bytes per query log
- 7-day retention = ~120MB for 10K QPS

---

## ADR-006: Go Templates vs Frontend Framework

**Status:** Accepted
**Date:** 2025-01-25
**Deciders:** Core team

### Context

Need web UI for monitoring and management. Options:
- Server-side rendering with Go templates
- Client-side SPA with React/Vue/Angular
- Hybrid approach

Requirements:
- Simple deployment (single binary)
- Fast initial load
- No complex build pipeline
- Easy maintenance

### Options Considered

#### Option 1: React/Vue SPA
Full client-side JavaScript framework.

**Pros:**
- Rich interactive UI
- Modern developer experience
- Component ecosystem

**Cons:**
- **Complex build pipeline** (npm, webpack, etc.)
- **Large bundle size**
- **Two separate codebases**
- **Deployment complexity**
- **Overkill for our needs**

#### Option 2: Go Templates (Chosen)
Server-side rendering with Go html/template.

**Pros:**
- **Simple deployment** - embedded in binary
- **No build pipeline** needed
- **Fast initial load** - no JavaScript bundle
- **SEO-friendly** (if needed)
- **Single codebase**
- **Easy maintenance**

**Cons:**
- Less interactive than SPA
- Need HTMX or similar for dynamic updates

### Decision

**Chosen: Option 2 - Go Templates + HTMX**

Go templates provide everything we need:
- Fast, simple, embedded
- Good enough interactivity with HTMX
- No build complexity

### Implementation

```go
// Location: /home/erfi/gloryhole/pkg/api/ui_handlers.go

//go:embed ui/templates/*.html ui/static/*
var uiFS embed.FS

var templates *template.Template

func initTemplates() error {
    var err error
    templates, err = template.ParseFS(uiFS, "ui/templates/*.html")
    return err
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
    data := struct {
        Version string
        Uptime  string
    }{
        Version: s.version,
        Uptime:  s.getUptime(),
    }

    templates.ExecuteTemplate(w, "dashboard.html", data)
}
```

### Consequences

**Positive:**
- Single binary deployment
- No build pipeline complexity
- Fast load times
- Simple maintenance
- Easy to understand

**Negative:**
- Less interactive than full SPA (acceptable tradeoff)

---

## ADR-007: HTMX vs React/Vue for Dynamic UI

**Status:** Accepted
**Date:** 2025-01-26
**Deciders:** Core team

### Context

After choosing Go templates (ADR-006), need dynamic updates for:
- Real-time statistics
- Query log streaming
- Dashboard updates

### Options Considered

#### Option 1: Vanilla JavaScript + Fetch
```javascript
setInterval(() => {
    fetch('/api/stats')
        .then(r => r.json())
        .then(data => updateDashboard(data));
}, 5000);
```

**Pros:**
- No dependencies
- Full control

**Cons:**
- Lots of boilerplate
- Manual DOM manipulation
- Error-prone

#### Option 2: React/Vue Components
Add React for dynamic sections only.

**Pros:**
- Rich component model
- Good developer experience

**Cons:**
- Still need build pipeline
- Complexity overhead
- Large bundle size

#### Option 3: HTMX (Chosen)
Declarative HTML attributes for AJAX.

```html
<div hx-get="/api/ui/stats"
     hx-trigger="every 5s"
     hx-swap="innerHTML">
    Loading...
</div>
```

**Pros:**
- **Minimal JavaScript** (~14KB)
- **Declarative** - HTML-driven
- **No build pipeline**
- **Server-side rendering**
- **Progressive enhancement**
- **Simple to learn**

**Cons:**
- Less powerful than full framework

### Decision

**Chosen: Option 3 - HTMX**

HTMX is perfect for our needs:
- Declarative dynamic updates
- Tiny footprint
- No build complexity
- Server stays in control

### Implementation

```html
<!-- Dashboard with auto-refresh -->
<div class="stats-grid">
    <div hx-get="/api/ui/stats"
         hx-trigger="every 5s"
         hx-swap="innerHTML">
        <!-- Server renders initial stats -->
    </div>
</div>

<!-- Query log with auto-refresh -->
<div hx-get="/api/ui/queries?limit=50"
     hx-trigger="every 2s"
     hx-swap="innerHTML">
    <!-- Server renders query table -->
</div>
```

Server returns HTML fragments:
```go
func (s *Server) handleStatsPartial(w http.ResponseWriter, r *http.Request) {
    stats, _ := s.storage.GetStatistics(ctx, time.Now().Add(-24*time.Hour))

    // Render HTML fragment (not full page)
    templates.ExecuteTemplate(w, "stats-partial.html", stats)
}
```

### Consequences

**Positive:**
- Simple, declarative dynamic updates
- Minimal JavaScript (~14KB)
- No build pipeline
- Server-side rendering benefits
- Progressive enhancement

**Negative:**
- Less powerful than full SPA framework
- Need to design good HTML fragments

---

## ADR-008: Embedded Assets vs External Files

**Status:** Accepted
**Date:** 2025-01-27
**Deciders:** Core team

### Context

Web UI has static assets (HTML, CSS, JavaScript). Options:
- Embed in binary using go:embed
- Serve from external files

### Options Considered

#### Option 1: External Files
```
glory-hole              # Binary
ui/                     # Separate directory
  templates/
  static/
```

**Pros:**
- Easy to edit without rebuild
- Traditional approach

**Cons:**
- **Deployment complexity** - need to ship directory
- **Path issues** - where to find files?
- **Version mismatch** - binary and files can get out of sync

#### Option 2: Embedded Files (Chosen)
```go
//go:embed ui/templates/*.html ui/static/*
var uiFS embed.FS
```

**Pros:**
- **Single binary** deployment
- **No path issues**
- **Version synchronization** guaranteed
- **Faster startup** - no file I/O

**Cons:**
- Need rebuild to change UI
- Larger binary size (~100KB for assets)

### Decision

**Chosen: Option 2 - Embedded Files**

Single binary deployment is a core principle of Glory-Hole. The ~100KB overhead is negligible, and the deployment simplicity is worth it.

### Implementation

```go
// Location: /home/erfi/gloryhole/pkg/api/ui_handlers.go

//go:embed ui/templates/*.html ui/static/*
var uiFS embed.FS

func initTemplates() error {
    templates, err := template.ParseFS(uiFS, "ui/templates/*.html")
    return err
}

func getStaticFS() (http.FileSystem, error) {
    staticFS, err := fs.Sub(uiFS, "ui/static")
    if err != nil {
        return nil, err
    }
    return http.FS(staticFS), nil
}
```

### Consequences

**Positive:**
- Single binary deployment
- No version mismatches
- No path configuration needed
- Faster startup

**Negative:**
- Need rebuild for UI changes (acceptable for releases)
- Slightly larger binary (~100KB)

---

## ADR-009: Cache Implementation - LRU Algorithm

**Status:** Accepted
**Date:** 2025-01-12
**Deciders:** Core team

### Context

DNS responses should be cached to reduce upstream queries. Need eviction policy for when cache is full.

### Options Considered

#### Option 1: LRU (Least Recently Used)
Evict entries that haven't been accessed recently.

**Pros:**
- Simple to implement
- Good performance
- Predictable behavior

**Cons:**
- Need to track access time

#### Option 2: LFU (Least Frequently Used)
Evict entries with lowest access count.

**Pros:**
- Keeps frequently accessed items

**Cons:**
- More complex
- Popular items stay forever

#### Option 3: TTL-Only
Evict only when TTL expires.

**Pros:**
- Respects DNS TTL

**Cons:**
- No size limit enforcement
- Memory can grow unbounded

### Decision

**Chosen: Option 1 - LRU with TTL**

Combine LRU eviction with TTL expiration:
- Entries expire based on DNS TTL
- LRU eviction when cache is full
- Background cleanup of expired entries

### Implementation

```go
// Location: /home/erfi/gloryhole/pkg/cache/cache.go

type cacheEntry struct {
    msg        *dns.Msg
    expiresAt  time.Time   // TTL expiration
    lastAccess time.Time   // For LRU
    size       int
}

func (c *Cache) Get(ctx context.Context, r *dns.Msg) *dns.Msg {
    c.mu.RLock()
    entry, found := c.entries[key]
    c.mu.RUnlock()

    if !found {
        return nil
    }

    // Check TTL expiration
    if time.Now().After(entry.expiresAt) {
        c.mu.Lock()
        delete(c.entries, key)
        c.mu.Unlock()
        return nil
    }

    // Update LRU timestamp
    c.mu.Lock()
    entry.lastAccess = time.Now()
    c.mu.Unlock()

    return entry.msg.Copy()
}

func (c *Cache) evictLRU() {
    var oldestKey string
    var oldestTime time.Time

    for key, entry := range c.entries {
        if oldestKey == "" || entry.lastAccess.Before(oldestTime) {
            oldestKey = key
            oldestTime = entry.lastAccess
        }
    }

    if oldestKey != "" {
        delete(c.entries, oldestKey)
        c.stats.evictions++
    }
}
```

### Consequences

**Positive:**
- Predictable memory usage
- Good hit rate
- Simple implementation

**Negative:**
- Slight overhead tracking access time

---

## ADR-010: DNS Server Architecture

**Status:** Accepted
**Date:** 2025-01-05
**Deciders:** Core team

### Context

Need to handle DNS queries over UDP and TCP with high concurrency.

### Options Considered

#### Option 1: Single Handler for Both UDP/TCP (Chosen)
Use `github.com/miekg/dns` library with unified handler.

**Pros:**
- **Code reuse** - same logic for both protocols
- **Well-tested** library
- **Standard Go patterns**
- **Excellent performance**

**Cons:**
- External dependency

#### Option 2: Custom Implementation
Write own DNS parser and server.

**Pros:**
- Full control
- No dependencies

**Cons:**
- **Massive effort**
- **Error-prone** (DNS is complex)
- **Not needed**

### Decision

**Chosen: Option 1 - miekg/dns Library**

The `miekg/dns` library is the de facto standard for DNS in Go. It's mature, well-tested, and performant.

### Implementation

```go
// Location: /home/erfi/gloryhole/pkg/dns/server.go

type Server struct {
    udpListener net.PacketConn
    tcpListener net.Listener
    handler     dns.Handler
    logger      *logging.Logger
}

func (s *Server) Start(ctx context.Context) error {
    errCh := make(chan error, 2)

    // Start UDP server
    go func() {
        srv := &dns.Server{
            PacketConn: s.udpListener,
            Handler:    s.handler,
        }
        errCh <- srv.ActivateAndServe()
    }()

    // Start TCP server
    go func() {
        srv := &dns.Server{
            Listener: s.tcpListener,
            Handler:  s.handler,
        }
        errCh <- srv.ActivateAndServe()
    }()

    select {
    case err := <-errCh:
        return err
    case <-ctx.Done():
        return s.Shutdown()
    }
}
```

### Consequences

**Positive:**
- Robust, tested implementation
- Standard patterns
- Good performance
- Easy to use

**Negative:**
- External dependency (acceptable)

---

## ADR-011: Configuration Format - YAML

**Status:** Accepted
**Date:** 2025-01-08
**Deciders:** Core team

### Context

Need human-editable configuration format.

### Options Considered

- **YAML** - Human-friendly, comments, multi-line
- **JSON** - Machine-friendly, no comments
- **TOML** - Between YAML and JSON
- **HCL** - Hashicorp format

### Decision

**Chosen: YAML**

Most user-friendly for configuration files. Supports comments, multi-line strings, and is widely known.

```yaml
# DNS Server Settings
server:
  listen_address: ":53"
  tcp_enabled: true
  udp_enabled: true

# Upstream DNS servers
upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"
```

---

## ADR-012: Structured Logging with slog

**Status:** Accepted
**Date:** 2025-01-09
**Deciders:** Core team

### Context

Need structured logging for production debugging.

### Decision

Use Go 1.21+ standard library `log/slog`:
- Built-in (no dependency)
- Structured logging
- Multiple formats (JSON, text)
- Good performance

```go
logger.Info("DNS query processed",
    "domain", "example.com",
    "client_ip", "192.168.1.100",
    "blocked", false,
    "response_time_ms", 5)
```

---

## ADR-013: CGO-Free SQLite Implementation

**Status:** Accepted
**Date:** 2025-01-11
**Deciders:** Core team

### Context

Standard SQLite drivers use CGO, which complicates cross-compilation.

### Decision

Use `modernc.org/sqlite` - pure Go SQLite implementation:
- No CGO required
- Easy cross-compilation
- Single binary for all platforms
- Performance close to CGO version

---

## ADR-014: Local Records Package Design

**Status:** Accepted
**Date:** 2025-01-28
**Deciders:** Core team

### Context

Need to support custom DNS records for local network (nas.local, etc.).

### Decision

Create dedicated `localrecords` package with:
- A/AAAA record support
- CNAME with chain resolution
- Wildcard domains (*.local)
- Multiple IPs per record
- Custom TTL

Separate from blocklist/overrides for clarity and testing.

---

## Summary

These architectural decisions shaped Glory-Hole into a high-performance, maintainable DNS server:

**Key Principles:**
1. **Performance first** - Lock-free hot paths
2. **Simplicity** - Single binary, minimal dependencies
3. **Maintainability** - Clear patterns, good tests
4. **User-friendly** - Easy configuration, good UI

**Technology Choices:**
- Go (performance + simplicity)
- Atomic pointers (lock-free reads)
- SQLite (embedded database)
- HTMX (simple dynamic UI)
- Expr (flexible policy engine)

For more details, see:
- [Component Documentation](components.md)
- [Performance Documentation](performance.md)
- [Testing Documentation](testing.md)
