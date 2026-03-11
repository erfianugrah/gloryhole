# Unbound Integration Plan

## Architecture

```
Client (:53) → Glory-Hole (filtering, policies, cache, local records)
                  ↓ allowed queries
              Unbound (127.0.0.1:5353, recursive resolution, DNSSEC, caching)
                  ↓ or forward-zone
              Upstream / Root servers
```

Glory-Hole remains the front-facing DNS server on port 53. Unbound runs as a
supervised child process on localhost:5353, handling recursive resolution and
DNSSEC validation. Glory-Hole's existing forwarder is replaced (or augmented)
with Unbound as the default upstream.

### What stays in Glory-Hole
- Port 53 listener (UDP/TCP/DoT)
- Policy engine (block/allow/redirect rules)
- Blocklist loading + matching
- Local records (synced → Unbound local-data for consistency)
- Query logging + analytics
- Web UI + HTTP API
- Client management

### What Unbound handles
- Recursive resolution (iterative queries to root/TLD/auth servers)
- DNSSEC validation
- Its own message/RRset cache (Glory-Hole cache can be disabled or act as L1)
- Forward zones (replaces Glory-Hole conditional forwarding upstreams)
- Stub zones (authoritative server delegation)
- Auth zones (locally hosted zones)
- RPZ (optional, complementary to Glory-Hole blocklists)
- Rate limiting (per-zone, per-IP)
- DNS-over-TLS upstream

---

## Phases

### Phase 1: Bundle Unbound in Docker image
**Goal**: Unbound runs as a supervised subprocess, Glory-Hole forwards to it.

#### 1.1 Dockerfile changes
- Install `unbound` package from Alpine repos (no need to compile from source,
  Alpine 3.21 ships Unbound 1.22.x)
- Install `unbound-libs` for `unbound-control`
- Create `/etc/unbound/` config directory
- Ship a default `unbound.conf` that:
  - Listens on `127.0.0.1@5353`
  - DNSSEC enabled with auto-trust-anchor
  - No access-control needed (localhost only)
  - remote-control enabled on localhost (for runtime management)
  - Sensible cache defaults (msg-cache-size: 64m, rrset-cache-size: 128m)

#### 1.2 Process supervision (`pkg/unbound/supervisor.go`)
- New package `pkg/unbound` with a `Supervisor` struct
- Starts Unbound via `exec.Command("unbound", "-d", "-c", configPath)`
- Captures stdout/stderr → Glory-Hole's logger
- Health checking via DNS probe to 127.0.0.1:5353
- Restart on crash with backoff
- Graceful shutdown: SIGTERM → wait → SIGKILL
- Config reload: `unbound-control reload` (no restart needed)

#### 1.3 Wire into Glory-Hole startup
- `cmd/glory-hole/main.go`: start Supervisor before DNS handler
- Default `upstream_dns_servers` to `["127.0.0.1:5353"]` when Unbound is enabled
- Config flag: `unbound.enabled: true` (default true in Docker, false for bare binary)
- Existing forwarder works unchanged — it just points at localhost:5353

#### 1.4 Config schema additions
```yaml
unbound:
  enabled: true
  binary_path: "/usr/sbin/unbound"    # auto-detected
  config_path: "/etc/unbound/unbound.conf"
  listen_port: 5353
```

**Deliverable**: Docker image with Unbound, Glory-Hole auto-forwards to it.
No UI changes yet. Users get recursive resolution + DNSSEC out of the box.

---

### Phase 2: Unbound config management (Go layer)
**Goal**: Go code that reads/writes/validates unbound.conf programmatically.

#### 2.1 Config model (`pkg/unbound/config.go`)
Go struct that maps to unbound.conf sections:

```go
type UnboundConfig struct {
    Server        ServerConfig         `json:"server"`
    ForwardZones  []ForwardZone        `json:"forward_zones"`
    StubZones     []StubZone           `json:"stub_zones"`
    AuthZones     []AuthZone           `json:"auth_zones"`
    RemoteControl RemoteControlConfig  `json:"remote_control"`
    RPZ           []RPZConfig          `json:"rpz"`
    Views         []ViewConfig         `json:"views"`
}

type ServerConfig struct {
    // Interface & Port
    Interfaces       []string `json:"interfaces"`
    Port             int      `json:"port"`
    OutgoingInterfaces []string `json:"outgoing_interfaces"`

    // Access Control
    AccessControl    []AccessControlEntry `json:"access_control"`

    // Cache
    MsgCacheSize     string `json:"msg_cache_size"`
    MsgCacheSlabs    int    `json:"msg_cache_slabs"`
    RRSetCacheSize   string `json:"rrset_cache_size"`
    RRSetCacheSlabs  int    `json:"rrset_cache_slabs"`
    CacheMaxTTL      int    `json:"cache_max_ttl"`
    CacheMinTTL      int    `json:"cache_min_ttl"`
    CacheMaxNegTTL   int    `json:"cache_max_negative_ttl"`
    InfraCacheNumHosts int  `json:"infra_cache_numhosts"`
    KeyCacheSize     string `json:"key_cache_size"`

    // DNSSEC
    ModuleConfig     string   `json:"module_config"`
    AutoTrustAnchor  string   `json:"auto_trust_anchor_file"`
    TrustAnchors     []string `json:"trust_anchors"`
    DomainInsecure   []string `json:"domain_insecure"`
    ValLogLevel      int      `json:"val_log_level"`
    ValPermissive    bool     `json:"val_permissive_mode"`
    HardenDNSSEC     bool     `json:"harden_dnssec_stripped"`
    HardenBelowNX    bool     `json:"harden_below_nxdomain"`
    HardenGlue       bool     `json:"harden_glue"`
    HardenAlgoDown   bool     `json:"harden_algo_downgrade"`

    // TLS
    TLSServiceKey    string `json:"tls_service_key"`
    TLSServicePem    string `json:"tls_service_pem"`
    TLSPort          int    `json:"tls_port"`
    TLSCertBundle    string `json:"tls_cert_bundle"`
    TLSUpstream      bool   `json:"tls_upstream"`

    // Logging
    Verbosity        int    `json:"verbosity"`
    LogQueries       bool   `json:"log_queries"`
    LogReplies       bool   `json:"log_replies"`
    LogServfail      bool   `json:"log_servfail"`

    // Performance
    NumThreads       int    `json:"num_threads"`
    SoReusePort      bool   `json:"so_reuseport"`
    OutgoingRange    int    `json:"outgoing_range"`
    NumQueriesPerThread int `json:"num_queries_per_thread"`
    EDNSBufferSize   int    `json:"edns_buffer_size"`

    // Hardening
    QnameMinimisation bool  `json:"qname_minimisation"`
    AggressiveNSEC    bool  `json:"aggressive_nsec"`

    // Serve Stale
    ServeExpired      bool  `json:"serve_expired"`
    ServeExpiredTTL   int   `json:"serve_expired_ttl"`
    ServeExpiredClientTimeout int `json:"serve_expired_client_timeout"`

    // Rate Limiting
    Ratelimit         int    `json:"ratelimit"`
    RatelimitSize     string `json:"ratelimit_size"`
    RatelimitFactor   int    `json:"ratelimit_factor"`
    IPRatelimit       int    `json:"ip_ratelimit"`
    IPRatelimitSize   string `json:"ip_ratelimit_size"`

    // Local Data
    LocalZones       []LocalZoneEntry `json:"local_zones"`
    LocalData        []string         `json:"local_data"`
}

type ForwardZone struct {
    Name           string   `json:"name"`
    ForwardAddrs   []string `json:"forward_addrs"`
    ForwardHosts   []string `json:"forward_hosts"`
    ForwardFirst   bool     `json:"forward_first"`
    ForwardTLS     bool     `json:"forward_tls_upstream"`
    ForwardNoCache bool     `json:"forward_no_cache"`
}

type StubZone struct {
    Name         string   `json:"name"`
    StubAddrs    []string `json:"stub_addrs"`
    StubHosts    []string `json:"stub_hosts"`
    StubPrime    bool     `json:"stub_prime"`
    StubFirst    bool     `json:"stub_first"`
    StubTLS      bool     `json:"stub_tls_upstream"`
}

type AuthZone struct {
    Name            string   `json:"name"`
    Primaries       []string `json:"primaries"`
    URLs            []string `json:"urls"`
    Zonefile        string   `json:"zonefile"`
    FallbackEnabled bool     `json:"fallback_enabled"`
    ForDownstream   bool     `json:"for_downstream"`
    ForUpstream     bool     `json:"for_upstream"`
    AllowNotify     []string `json:"allow_notify"`
}

type RPZConfig struct {
    Name           string `json:"name"`
    Zonefile       string `json:"zonefile"`
    URL            string `json:"url"`
    Primary        string `json:"primary"`
    ActionOverride string `json:"rpz_action_override"`
    CNAMEOverride  string `json:"rpz_cname_override"`
    Log            bool   `json:"rpz_log"`
    LogName        string `json:"rpz_log_name"`
}
```

#### 2.2 Config serializer (`pkg/unbound/writer.go`)
- `WriteConfig(cfg *UnboundConfig, path string) error`
- Generates valid unbound.conf from the Go struct
- Atomic write (temp + rename, same as Glory-Hole config)

#### 2.3 Config parser (`pkg/unbound/parser.go`)
- `ParseConfig(path string) (*UnboundConfig, error)`
- Reads existing unbound.conf back into Go struct
- Handles include directives

#### 2.4 Config validator
- `Validate(cfg *UnboundConfig) error`
- Or shell out to `unbound-checkconf <path>` for authoritative validation

---

### Phase 3: API endpoints
**Goal**: Full CRUD for Unbound config sections via REST API.

#### 3.1 Read endpoints
```
GET /api/unbound/config          → full UnboundConfig JSON
GET /api/unbound/config/server   → server section
GET /api/unbound/forward-zones   → list of forward zones
GET /api/unbound/stub-zones      → list of stub zones
GET /api/unbound/auth-zones      → list of auth zones
GET /api/unbound/rpz             → list of RPZ configs
GET /api/unbound/status          → unbound-control status output
GET /api/unbound/stats           → unbound-control stats (cache hits, etc.)
```

#### 3.2 Write endpoints
```
PUT  /api/unbound/config/server     → update server config
PUT  /api/unbound/config/dnssec     → update DNSSEC settings
PUT  /api/unbound/config/cache      → update cache settings
PUT  /api/unbound/config/tls        → update TLS settings
PUT  /api/unbound/config/logging    → update logging settings
PUT  /api/unbound/config/performance → update threads/buffers
PUT  /api/unbound/config/ratelimit  → update rate limiting
PUT  /api/unbound/config/hardening  → update hardening flags

POST   /api/unbound/forward-zones   → add forward zone
PUT    /api/unbound/forward-zones/:name → update forward zone
DELETE /api/unbound/forward-zones/:name → delete forward zone

POST   /api/unbound/stub-zones      → add stub zone
PUT    /api/unbound/stub-zones/:name → update stub zone
DELETE /api/unbound/stub-zones/:name → delete stub zone

POST   /api/unbound/auth-zones      → add auth zone
PUT    /api/unbound/auth-zones/:name → update auth zone
DELETE /api/unbound/auth-zones/:name → delete auth zone

POST   /api/unbound/rpz             → add RPZ
PUT    /api/unbound/rpz/:name       → update RPZ
DELETE /api/unbound/rpz/:name       → delete RPZ
```

#### 3.3 Control endpoints
```
POST /api/unbound/reload            → unbound-control reload
POST /api/unbound/flush-cache       → unbound-control flush_zone .
POST /api/unbound/flush-zone/:zone  → unbound-control flush_zone <zone>
```

#### 3.4 Apply pattern
All write endpoints follow:
1. Parse + validate request body
2. Load current UnboundConfig
3. Apply changes
4. Validate via `unbound-checkconf`
5. Write config atomically
6. Reload via `unbound-control reload`
7. Return updated config section

---

### Phase 4: UI — Unbound settings pages
**Goal**: Full Unbound config management in the dashboard.

#### 4.1 Navigation
Add "Resolver" collapsible section to sidebar:
- **Resolver Overview** — status, stats (cache hit rate, queries, uptime)
- **Server Config** — interface, threads, buffers, hardening toggles
- **DNSSEC** — module config, trust anchors, insecure domains, validation mode
- **Cache** — sizes, slabs, TTL ranges, serve-stale settings
- **Forward Zones** — CRUD table (name, addrs, TLS, first, no-cache)
- **Stub Zones** — CRUD table
- **Auth Zones** — CRUD table (with zonefile upload?)
- **RPZ** — CRUD table (sources, actions, logging)
- **Rate Limiting** — per-zone and per-IP settings
- **TLS** — upstream TLS, service TLS, certs
- **Logging** — verbosity, query/reply logging

#### 4.2 Component structure
```
src/components/
  unbound/
    UnboundOverviewPage.tsx    — stats dashboard (cache hits, thread load)
    UnboundServerPage.tsx      — server section form
    UnboundDNSSECPage.tsx      — DNSSEC config form
    UnboundCachePage.tsx       — cache settings form
    UnboundForwardZonesPage.tsx — forward zone CRUD table + dialog
    UnboundStubZonesPage.tsx   — stub zone CRUD table + dialog
    UnboundAuthZonesPage.tsx   — auth zone CRUD table + dialog
    UnboundRPZPage.tsx         — RPZ CRUD table + dialog
    UnboundRateLimitPage.tsx   — rate limit form
    UnboundTLSPage.tsx         — TLS config form
    UnboundLoggingPage.tsx     — logging config form
```

Each page follows the existing pattern: `useEffect` fetch on mount, form state
via `useState`, save via PUT/POST, toast feedback.

#### 4.3 Astro pages
```
src/pages/
  resolver/
    index.astro          → UnboundOverviewPage
    server.astro         → UnboundServerPage
    dnssec.astro         → UnboundDNSSECPage
    cache.astro          → UnboundCachePage
    forward-zones.astro  → UnboundForwardZonesPage
    stub-zones.astro     → UnboundStubZonesPage
    auth-zones.astro     → UnboundAuthZonesPage
    rpz.astro            → UnboundRPZPage
    ratelimit.astro      → UnboundRateLimitPage
    tls.astro            → UnboundTLSPage
    logging.astro        → UnboundLoggingPage
```

---

### Phase 5: Local record sync + conditional forwarding migration
**Goal**: Unify overlapping features.

#### 5.1 Local records → Unbound local-data
When Glory-Hole local records are added/modified/deleted, also update Unbound's
local-data via `unbound-control local_data <name> <type> <data>` or config
reload. This ensures Unbound's cache is consistent.

#### 5.2 Conditional forwarding → Unbound forward-zones
Glory-Hole's conditional forwarding rules can map to Unbound forward-zones.
Options:
- **Keep both**: Glory-Hole rules for complex matching (CIDR, qtype),
  Unbound forward-zones for simple domain-based forwarding
- **Migrate**: Convert simple domain rules to Unbound forward-zones,
  keep Glory-Hole rules only for CIDR/qtype matching

#### 5.3 Cache strategy
With Unbound handling caching:
- Option A: Disable Glory-Hole cache, let Unbound be the sole cache
- Option B: Glory-Hole cache = L1 (small, fast), Unbound cache = L2 (large, DNSSEC-validated)
- Recommendation: **Option A** for simplicity — Unbound's cache is battle-tested

---

### Phase 6: Non-Docker support
**Goal**: Unbound integration works outside Docker too.

- Auto-detect Unbound binary in PATH
- Config flag `unbound.binary_path` for custom location
- `unbound.manage_config: false` for users who manage unbound.conf themselves
  (Glory-Hole just forwards to it, no config writes)
- SystemD unit file template for running both services
- Bare metal install docs

---

## Priority order

1. **Phase 1** — Bundle + supervise (biggest user-facing value, enables DNSSEC)
2. **Phase 2** — Config management (Go layer, foundation for everything else)
3. **Phase 3** — API endpoints (enables UI)
4. **Phase 4** — UI pages (user-facing config)
5. **Phase 5** — Feature sync (polish, dedup)
6. **Phase 6** — Non-Docker (broader audience)

## Open questions

1. **Cache dedup**: Should we disable Glory-Hole's cache entirely when Unbound
   is enabled, or keep it as L1?
2. **Blocklist format**: Should we also feed Glory-Hole blocklists to Unbound
   as RPZ/local-zone for defense-in-depth, or keep blocking purely in Glory-Hole?
3. **Config ownership**: If a user edits unbound.conf manually, should Glory-Hole
   detect drift and warn, or just overwrite on next save?
4. **Upgrade path**: For users running klutchell/unbound separately, how do we
   migrate their existing unbound.conf into the managed config?
