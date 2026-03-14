# Unbound Integration Plan

## Architecture

```
Client (:53) → Glory-Hole (filtering, policies, local records, query logging)
                  ↓ allowed queries
              Unbound (127.0.0.1:5353, recursive resolution, DNSSEC, caching)
                  ↓ recursive or forward-zone
              Upstream / Root servers
```

Glory-Hole remains the front-facing DNS server on port 53. Unbound runs as a
supervised child process on localhost:5353, handling recursive resolution,
DNSSEC validation, and caching. Glory-Hole's existing forwarder points at
Unbound instead of external upstreams.

### What stays in Glory-Hole
- Port 53 listener (UDP/TCP/DoT)
- Policy engine (block/allow/redirect rules)
- Blocklist loading + matching
- Local records (highest priority, also pushed to Unbound as local-data)
- Query logging + analytics
- Web UI + HTTP API
- Client management
- Conditional forwarding for CIDR/qtype-based rules

### What Unbound handles
- Recursive resolution (iterative queries to root/TLD/auth servers)
- DNSSEC validation
- **All caching** (Glory-Hole cache disabled when Unbound is active)
- Forward zones (domain-based forwarding, replaces simple domain-only rules)
- Stub zones (authoritative server delegation)
- Auth zones (locally hosted zones)
- RPZ (optional, complementary to Glory-Hole blocklists)
- Rate limiting (per-zone, per-IP)
- DNS-over-TLS upstream

### What gets disabled when Unbound is active
- **Simple domain-only conditional forwarding rules** — migrated to Unbound
  forward-zones. Glory-Hole conditional forwarding remains for CIDR/qtype rules.
- Glory-Hole's cache and policy engine remain fully active.

---

## Decisions

### Cache: Glory-Hole's cache stays active
When Unbound is enabled, Glory-Hole's cache remains the primary cache. Reasons:
- Glory-Hole's cache is purgeable via API (`/api/cache/purge`) and UI.
- Cache stats are already wired into Prometheus metrics and the dashboard.
- Blocklist-aware caching: `SetBlocked()` uses custom TTLs that Unbound
  has no concept of.
- Policy decisions are never cached (by design) — this invariant is already
  enforced in the handler.
- DNSSEC correctness is maintained: Unbound validates DNSSEC *before*
  returning the response, so cached entries were validated at insertion time.
  TTLs from upstream are respected, so entries expire at the same time.
- Unbound's own cache acts as a warm L2 behind Glory-Hole for recursive
  queries that miss both caches. This is beneficial, not redundant.

### Blocklists: Stay in Glory-Hole only
Blocking stays in Glory-Hole's pipeline (policy engine + blocklist manager).
No RPZ duplication. Unbound RPZ is exposed in the UI for advanced users who
want additional server-side filtering, but it's not auto-populated from
Glory-Hole's blocklists.

### Config ownership: Glory-Hole owns unbound.conf
Glory-Hole always writes the full `unbound.conf` from its Go model. No
parser for reading existing configs. Manual edits to `unbound.conf` will be
overwritten on next save. For users with existing configs, provide a one-time
migration guide (copy forward-zone blocks into the UI).

### Conditional forwarding model
- **CIDR/qtype rules** → stay in Glory-Hole's conditional forwarding handler.
  These are evaluated in Go before forwarding to the matched upstream.
- **Domain-only rules** → configured as Unbound forward-zones via the UI.
  Unbound handles the forwarding natively (supports TLS, TCP, forward-first
  fallback to recursion).
- The UI makes this clear: "Domain forwarding" section configures Unbound
  forward-zones. "Advanced forwarding" section configures Glory-Hole rules
  for CIDR/qtype matching.

### Feature visibility when Unbound is disabled
- All `/api/unbound/*` endpoints return `503 Service Unavailable`
- The "Resolver" nav section is hidden in the sidebar
- Glory-Hole's cache and forwarder work exactly as before
- No Unbound-related config appears in the settings

### Managed vs external mode (decided upfront, not deferred)
Two modes are defined from Phase 1 because the mode affects how every
other component behaves:
- **`managed: true`** (default in Docker) — Glory-Hole starts/stops/
  supervises Unbound, writes `unbound.conf`, handles DNSSEC anchor
  bootstrapping, monitors health, restarts on crash.
- **`managed: false`** — User manages Unbound externally (systemd, etc.).
  Glory-Hole only forwards to it. No config writes, no supervision. Stats
  endpoints still work if the control socket is accessible.

---

## Failure modes & recovery

### Unbound fails to start on boot
1. Supervisor logs the error (with stderr from Unbound).
2. **Fallback**: If `unbound.fallback_upstreams` is set (or previous
   `upstream_dns_servers` config exists), Glory-Hole falls back to simple
   forwarding mode with its own cache re-enabled. DNS resolution continues
   (without DNSSEC/recursion).
3. Supervisor retries in background with exponential backoff (1s→2s→4s→max 30s).
4. API reports `GET /api/unbound/status` as `degraded` with the error.
5. When Unbound eventually starts, supervisor transitions to normal mode,
   disables Glory-Hole cache again, and switches forwarder to 127.0.0.1:5353.

### Unbound crashes during operation
1. Supervisor detects process exit, logs exit code + stderr.
2. Immediate restart attempt (no backoff for first retry).
3. During the restart window (typically <1s), queries forwarded to Unbound
   will fail. Glory-Hole's forwarder circuit breaker opens after threshold
   failures, triggering the fallback upstreams if configured.
4. If 5 consecutive restart attempts fail within 60 seconds, supervisor
   enters `failed` state and falls back to forwarding mode (same as boot
   failure). Manual intervention or config fix required.

### unbound-checkconf rejects a config change
1. API write endpoint returns 400 with the `unbound-checkconf` error message.
2. No changes are persisted to `config.yml`.
3. No reload is sent to Unbound.
4. The running Unbound continues with the previous valid config.
5. UI shows the validation error to the user.

### unbound-control reload fails
1. `unbound.conf` was already written (it passed `unbound-checkconf`).
2. API returns 500, logs the error.
3. The stale `unbound.conf` on disk is valid but not yet loaded by Unbound.
4. Next restart of Unbound will pick up the new config.
5. UI shows a warning: "Config saved but reload failed. Restart Unbound to
   apply changes."

---

## Phases

### Phase 1: Bundle Unbound in Docker image
**Goal**: Unbound runs as a supervised subprocess, Glory-Hole forwards to it.
Users get recursive resolution + DNSSEC out of the box with zero config.

#### 1.1 Dockerfile changes — Build Unbound from source

Both `Dockerfile` (dev) and `Dockerfile.release` (CI) add an Unbound build
stage. Building from source gives us exact version pinning, a static binary
with no runtime shared library dependencies, and a smaller final image.

**Unbound build stage** (runs in parallel with Go build):

```dockerfile
# Stage: Build Unbound from source
FROM alpine:3.21 AS unbound-builder

ARG UNBOUND_VERSION=1.24.2

RUN apk add --no-cache build-base openssl-dev libexpat expat-dev libevent-dev curl

RUN curl -fsSL "https://nlnetlabs.nl/downloads/unbound/unbound-${UNBOUND_VERSION}.tar.gz" \
        -o unbound.tar.gz && \
    tar xzf unbound.tar.gz

WORKDIR /unbound-${UNBOUND_VERSION}

RUN ./configure \
        --prefix=/opt/unbound \
        --with-libevent \
        --with-ssl \
        --disable-shared \
        --disable-flto \
        --without-pythonmodule \
        --without-pyunbound && \
    make -j$(nproc) && \
    make install

# Fetch fresh root hints
RUN curl -fsSL https://www.internic.net/domain/named.root \
        -o /opt/unbound/etc/unbound/root.hints
```

**Runtime stage additions** (in both Dockerfiles):

```dockerfile
# Copy Unbound static binaries from build stage
COPY --from=unbound-builder /opt/unbound/sbin/unbound /usr/local/bin/unbound
COPY --from=unbound-builder /opt/unbound/sbin/unbound-control /usr/local/bin/unbound-control
COPY --from=unbound-builder /opt/unbound/sbin/unbound-checkconf /usr/local/bin/unbound-checkconf
COPY --from=unbound-builder /opt/unbound/sbin/unbound-anchor /usr/local/bin/unbound-anchor

# Copy default config and root hints
COPY deploy/unbound/unbound.conf /etc/unbound/unbound.conf
COPY --from=unbound-builder /opt/unbound/etc/unbound/root.hints /etc/unbound/root.hints

# Create Unbound runtime directories
RUN mkdir -p /etc/unbound/custom.conf.d /var/run/unbound && \
    chown -R glory-hole:glory-hole /etc/unbound /var/run/unbound
```

**Build characteristics**:
- Dynamically linked against `libssl`, `libevent`, `libexpat` (shared libs
  added to runtime stage via `apk add libevent libexpat` — OpenSSL is already
  present from `ca-certificates`).
- Compile time: ~60-90s on CI runners.
- Binary size: ~5-6MB (unbound + unbound-control + unbound-checkconf + unbound-anchor).
- Runtime deps added to final image: `libevent` (~0.5MB) + `libexpat` (~0.2MB).
- Total image size delta: ~7MB (binaries + shared libs + root.hints + config).
- Compare to `apk add unbound`: ~15-20MB (pulls full package + all transitive deps).

**Version pinning**: `UNBOUND_VERSION` is a build arg, pinned in the
Dockerfile. NLnet Labs publishes signed tarballs at a predictable URL.
Update the version in one place when upgrading.

**No `setcap` needed for Unbound**: It binds only to 127.0.0.1:5353
(unprivileged port, localhost only). Only Glory-Hole needs `cap_net_bind_service`
for port 53.

#### 1.2 Entrypoint changes (`docker-entrypoint.sh`)

The entrypoint stays minimal. DNSSEC anchor bootstrapping is handled by the
Go supervisor (not the shell script) so it also works in non-Docker deployments.

Only change: ensure Unbound directories have correct ownership when running
as root (same pattern as existing `chown` for app directories):

```sh
# Add after existing chown block (before exec):
if [ "$(id -u)" = "0" ]; then
    chown -R glory-hole:glory-hole /etc/unbound /var/run/unbound 2>/dev/null || true
fi
```

#### 1.3 Process supervision (`pkg/unbound/supervisor.go`)

```go
type Supervisor struct {
    cmd           *exec.Cmd
    cfg           SupervisorConfig
    logger        *slog.Logger
    mu            sync.Mutex
    state         State         // stopped | starting | running | degraded | failed
    lastErr       error
    restartCount  int
    cancelHealth  context.CancelFunc
}

type SupervisorConfig struct {
    BinaryPath    string // auto-detected if empty
    ConfigPath    string // /etc/unbound/unbound.conf
    ControlSocket string // /var/run/unbound/control.sock
    ListenAddr    string // 127.0.0.1:5353
    // Paths to unbound-control and unbound-checkconf (auto-detected)
    ControlBin    string
    CheckconfBin  string
}

type State string
const (
    StateStopped  State = "stopped"
    StateStarting State = "starting"
    StateRunning  State = "running"
    StateDegraded State = "degraded" // started but health checks failing
    StateFailed   State = "failed"   // gave up restarting
)
```

**Startup sequence** (called from `main.go`):
1. Auto-detect binary paths (`unbound`, `unbound-control`, `unbound-checkconf`)
   via `exec.LookPath`, then fallback to common paths.
2. Run `unbound-anchor -a /etc/unbound/root.key` to bootstrap/refresh the
   DNSSEC root trust anchor. This is idempotent and safe to run every startup.
   Failure is non-fatal (logged as warning — anchor may already exist).
3. Write default `unbound.conf` if none exists (or regenerate from config).
4. Run `unbound-checkconf <configPath>` to validate before starting.
5. Start Unbound: `exec.Command(binaryPath, "-d", "-c", configPath)`.
   - `-d` = don't daemonize (stays in foreground, supervisor manages lifecycle)
   - Capture stdout/stderr → slog with `[unbound]` prefix
6. **Readiness gate**: Probe `127.0.0.1:5353` with a DNS `CH TXT id.server`
   query in a loop (100ms interval, 30s timeout). If timeout: log error,
   enter `failed` state, trigger fallback.
7. Start health monitor goroutine (every 10s DNS probe).

**Orphan detection** (before step 5):
- Check if port 5353 is already listening (`net.DialTimeout`).
- If so, try `unbound-control status` via the socket.
- If it responds, adopt it (skip starting a new process, go to step 7).
- If it doesn't respond, fail with: "Port 5353 in use by unknown process".

**Health monitoring**:
- Every 10s: send `CH TXT id.server` to 127.0.0.1:5353.
- 3 consecutive failures → restart Unbound with backoff (1s, 2s, 4s, max 30s).
- 5 restarts within 60s → enter `failed` state, stop retrying.
- Health status exposed via `Status() (State, error)`.

**Reload** (config changes without restart):
```go
func (s *Supervisor) Reload() error {
    return s.runControl("reload")
}

func (s *Supervisor) FlushCache() error {
    return s.runControl("flush_zone", ".")
}

func (s *Supervisor) FlushZone(zone string) error {
    return s.runControl("flush_zone", zone)
}

func (s *Supervisor) runControl(args ...string) error {
    cmdArgs := append([]string{"-s", s.cfg.ControlSocket}, args...)
    out, err := exec.Command(s.cfg.ControlBin, cmdArgs...).CombinedOutput()
    if err != nil {
        return fmt.Errorf("unbound-control %v: %w: %s", args, err, out)
    }
    return nil
}
```

**Shutdown**: SIGTERM → 5s grace → SIGKILL. Called from Glory-Hole's shutdown
handler before storage/telemetry cleanup.

#### 1.4 Wire into Glory-Hole startup (`cmd/glory-hole/main.go`)

Insert between config watcher start (step 4) and DNS handler creation (step 7):

```go
// After config watcher start, before DNS handler creation:
var unboundSupervisor *unbound.Supervisor

if cfg.Unbound.Enabled && cfg.Unbound.Managed {
    ubCfg := unbound.SupervisorConfig{
        BinaryPath:    cfg.Unbound.BinaryPath,
        ConfigPath:    cfg.Unbound.ConfigPath,
        ControlSocket: cfg.Unbound.ControlSocket,
        ListenAddr:    fmt.Sprintf("127.0.0.1:%d", cfg.Unbound.ListenPort),
    }

    unboundSupervisor = unbound.NewSupervisor(ubCfg, logger)

    // Write unbound.conf from config (or default)
    if cfg.Unbound.Config != nil {
        if err := unbound.WriteConfig(cfg.Unbound.Config, cfg.Unbound.ConfigPath); err != nil {
            logger.Error("Failed to write unbound.conf", "error", err)
            // Continue — default config from image may still work
        }
    }

    if err := unboundSupervisor.Start(ctx); err != nil {
        logger.Error("Unbound failed to start", "error", err)
        // Fallback: keep upstream_dns_servers and cache as-is
    } else {
        // Override upstreams — cache stays active for API purge, metrics,
        // and blocklist-aware caching
        cfg.UpstreamDNSServers = []string{ubCfg.ListenAddr}
        logger.Info("Unbound active, forwarding to local resolver",
            "addr", ubCfg.ListenAddr)
    }
}
```

The existing `forwarder.NewForwarder(cfg, logger)` call (inside
`dns.NewServer` at line 560) will automatically use `["127.0.0.1:5353"]`
since we mutated `cfg.UpstreamDNSServers`.

**Hot-reload callback** (main.go lines 582-854) additions:

```go
// In the OnChange callback:
// 1. If unbound.enabled changed true→false:
//    - Stop supervisor
//    - Restore upstream_dns_servers from new config
//    - Re-enable cache
//    - Create new forwarder

// 2. If unbound.enabled changed false→true:
//    - Start supervisor
//    - Override upstreams to 127.0.0.1:5353
//    - Disable cache
//    - Create new forwarder pointing at Unbound

// 3. If unbound config changed (but still enabled):
//    - Write new unbound.conf
//    - Validate with unbound-checkconf
//    - Reload via unbound-control
//    - No forwarder/cache changes needed
```

**Shutdown order** (main.go lines 886-927) addition:

```go
// After stopping DNS server and API server, before storage close:
if unboundSupervisor != nil {
    logger.Info("Stopping Unbound resolver")
    unboundSupervisor.Stop() // SIGTERM → wait → SIGKILL
}
```

#### 1.5 Config schema additions

```yaml
unbound:
  enabled: true                    # default: true in Docker, false for bare binary
  managed: true                    # true = supervise process, false = external
  binary_path: ""                  # auto-detected from PATH if empty
  config_path: "/etc/unbound/unbound.conf"
  listen_port: 5353
  control_socket: "/var/run/unbound/control.sock"
  fallback_upstreams:              # used when Unbound fails (optional)
    - "1.1.1.1:53"
    - "8.8.8.8:53"
```

Added to `pkg/config/config.go`:
```go
type UnboundConfig struct {
    Enabled           bool         `yaml:"enabled"`
    Managed           bool         `yaml:"managed"`
    BinaryPath        string       `yaml:"binary_path"`
    ConfigPath        string       `yaml:"config_path"`
    ListenPort        int          `yaml:"listen_port"`
    ControlSocket     string       `yaml:"control_socket"`
    FallbackUpstreams []string     `yaml:"fallback_upstreams"`
    Config            *UnboundServerConfig `yaml:"config,omitempty"` // Phase 2
}
```

Default values (set in `config.Load` or `config.Defaults()`):
- `enabled: true` in Docker (detected via `/etc/glory-hole/.docker` sentinel
  file or `GLORY_HOLE_DOCKER=1` env var)
- `enabled: false` for bare binary
- `managed: true`
- `listen_port: 5353`
- `config_path: /etc/unbound/unbound.conf`
- `control_socket: /var/run/unbound/control.sock`

#### 1.6 Default unbound.conf

Shipped in the Docker image at `/etc/unbound/unbound.conf`. Used when no
custom config is present. After Phase 2, this file is always generated from
the Go model.

```
server:
    interface: 127.0.0.1@5353
    do-ip6: no
    do-daemonize: no
    username: ""
    chroot: ""
    directory: "/etc/unbound"

    # Access control (localhost only)
    access-control: 127.0.0.0/8 allow
    access-control: 0.0.0.0/0 refuse

    # DNSSEC
    module-config: "validator iterator"
    auto-trust-anchor-file: "/etc/unbound/root.key"

    # Cache
    msg-cache-size: 64m
    rrset-cache-size: 128m
    cache-max-ttl: 86400
    cache-min-ttl: 0
    cache-max-negative-ttl: 3600
    infra-cache-numhosts: 10000
    key-cache-size: 8m

    # Performance
    num-threads: 2
    so-reuseport: yes
    edns-buffer-size: 1232

    # Hardening
    harden-glue: yes
    harden-dnssec-stripped: yes
    harden-below-nxdomain: yes
    harden-algo-downgrade: yes
    qname-minimisation: yes
    aggressive-nsec: yes

    # Serve stale
    serve-expired: yes
    serve-expired-ttl: 86400
    serve-expired-client-timeout: 1800

    # Logging (minimal — Glory-Hole handles query logging)
    verbosity: 1
    logfile: ""
    log-queries: no
    log-replies: no
    log-servfail: yes

remote-control:
    control-enable: yes
    control-interface: /var/run/unbound/control.sock
    control-use-cert: no
```

#### 1.7 Deliverable
Docker image with Unbound. `docker run` gives recursive resolution + DNSSEC
with zero config. If Unbound fails to start, falls back to simple forwarding
(existing behavior). Users who set `unbound.enabled: false` get exactly the
old behavior with no overhead.

---

### Phase 2: Unbound config model + serializer
**Goal**: Go code that generates valid `unbound.conf` from a typed struct,
validated by `unbound-checkconf`.

No parser. Glory-Hole owns the config and always writes the full file.

#### 2.1 Config model — two tiers

The config model is split into **essential** (exposed in UI) and **advanced**
(config.yml only) tiers. The Go struct contains all fields, but the UI only
renders forms for the essential tier. Advanced users edit `config.yml` directly
for the rest.

**Essential** (shown in UI):

| Category | Fields |
|----------|--------|
| Cache | msg_cache_size, rrset_cache_size, cache_max_ttl, cache_min_ttl, cache_max_negative_ttl |
| DNSSEC | module_config (on/off/permissive), domain_insecure list, harden_dnssec_stripped |
| Performance | num_threads |
| Hardening | qname_minimisation, aggressive_nsec, harden_glue, harden_below_nxdomain |
| Serve Stale | serve_expired toggle, serve_expired_ttl |
| Logging | verbosity (0-5) |
| Forward Zones | full CRUD (name, addrs, TLS, forward-first) |
| Stub Zones | full CRUD (name, addrs, prime, first) |

**Advanced** (config.yml only, not in UI):

| Category | Fields |
|----------|--------|
| Cache | msg_cache_slabs, rrset_cache_slabs, infra_cache_numhosts, key_cache_size |
| Performance | so_reuseport, outgoing_range, num_queries_per_thread, edns_buffer_size |
| Rate Limiting | ratelimit, ratelimit_size, ratelimit_factor, ip_ratelimit, ip_ratelimit_size |
| TLS | tls_cert_bundle, tls_upstream, tls_service_key/pem, tls_port |
| Auth Zones | full config |
| RPZ | full config |
| Access Control | custom ACL entries |

This keeps the UI manageable (~15 settings) while supporting the full
Unbound feature set via config.yml.

#### 2.2 Config struct (`pkg/unbound/config.go`)

```go
// UnboundServerConfig is the full typed representation of unbound.conf.
// Fields with `ui:"essential"` are exposed in the web UI.
// All other fields are config.yml-only.
type UnboundServerConfig struct {
    Server        ServerBlock       `yaml:"server" json:"server"`
    ForwardZones  []ForwardZone     `yaml:"forward_zones" json:"forward_zones"`
    StubZones     []StubZone        `yaml:"stub_zones" json:"stub_zones"`
    AuthZones     []AuthZone        `yaml:"auth_zones,omitempty" json:"auth_zones,omitempty"`
    RemoteControl RemoteControl     `yaml:"remote_control" json:"remote_control"`
    RPZ           []RPZConfig       `yaml:"rpz,omitempty" json:"rpz,omitempty"`
}

type ServerBlock struct {
    // Cache (essential)
    MsgCacheSize       string `yaml:"msg_cache_size" json:"msg_cache_size"`
    RRSetCacheSize     string `yaml:"rrset_cache_size" json:"rrset_cache_size"`
    CacheMaxTTL        int    `yaml:"cache_max_ttl" json:"cache_max_ttl"`
    CacheMinTTL        int    `yaml:"cache_min_ttl" json:"cache_min_ttl"`
    CacheMaxNegTTL     int    `yaml:"cache_max_negative_ttl" json:"cache_max_negative_ttl"`

    // Cache (advanced)
    MsgCacheSlabs      int    `yaml:"msg_cache_slabs,omitempty" json:"msg_cache_slabs,omitempty"`
    RRSetCacheSlabs    int    `yaml:"rrset_cache_slabs,omitempty" json:"rrset_cache_slabs,omitempty"`
    InfraCacheNumHosts int    `yaml:"infra_cache_numhosts,omitempty" json:"infra_cache_numhosts,omitempty"`
    KeyCacheSize       string `yaml:"key_cache_size,omitempty" json:"key_cache_size,omitempty"`

    // DNSSEC (essential)
    ModuleConfig       string   `yaml:"module_config" json:"module_config"`
    DomainInsecure     []string `yaml:"domain_insecure,omitempty" json:"domain_insecure,omitempty"`
    HardenDNSSEC       bool     `yaml:"harden_dnssec_stripped" json:"harden_dnssec_stripped"`

    // DNSSEC (advanced)
    AutoTrustAnchor    string `yaml:"auto_trust_anchor_file" json:"auto_trust_anchor_file"`
    ValLogLevel        int    `yaml:"val_log_level,omitempty" json:"val_log_level,omitempty"`
    ValPermissive      bool   `yaml:"val_permissive_mode,omitempty" json:"val_permissive_mode,omitempty"`
    HardenAlgoDown     bool   `yaml:"harden_algo_downgrade" json:"harden_algo_downgrade"`

    // Hardening (essential)
    HardenGlue         bool `yaml:"harden_glue" json:"harden_glue"`
    HardenBelowNX      bool `yaml:"harden_below_nxdomain" json:"harden_below_nxdomain"`
    QnameMinimisation  bool `yaml:"qname_minimisation" json:"qname_minimisation"`
    AggressiveNSEC     bool `yaml:"aggressive_nsec" json:"aggressive_nsec"`

    // Performance (essential: threads only)
    NumThreads         int  `yaml:"num_threads" json:"num_threads"`

    // Performance (advanced)
    SoReusePort        bool `yaml:"so_reuseport,omitempty" json:"so_reuseport,omitempty"`
    OutgoingRange      int  `yaml:"outgoing_range,omitempty" json:"outgoing_range,omitempty"`
    NumQueriesPerThread int `yaml:"num_queries_per_thread,omitempty" json:"num_queries_per_thread,omitempty"`
    EDNSBufferSize     int  `yaml:"edns_buffer_size,omitempty" json:"edns_buffer_size,omitempty"`

    // Serve Stale (essential)
    ServeExpired              bool `yaml:"serve_expired" json:"serve_expired"`
    ServeExpiredTTL           int  `yaml:"serve_expired_ttl" json:"serve_expired_ttl"`
    ServeExpiredClientTimeout int  `yaml:"serve_expired_client_timeout,omitempty" json:"serve_expired_client_timeout,omitempty"`

    // Logging (essential: verbosity only)
    Verbosity  int  `yaml:"verbosity" json:"verbosity"`
    LogQueries bool `yaml:"log_queries,omitempty" json:"log_queries,omitempty"`
    LogReplies bool `yaml:"log_replies,omitempty" json:"log_replies,omitempty"`
    LogServfail bool `yaml:"log_servfail,omitempty" json:"log_servfail,omitempty"`

    // Rate Limiting (advanced)
    Ratelimit       int    `yaml:"ratelimit,omitempty" json:"ratelimit,omitempty"`
    RatelimitSize   string `yaml:"ratelimit_size,omitempty" json:"ratelimit_size,omitempty"`
    IPRatelimit     int    `yaml:"ip_ratelimit,omitempty" json:"ip_ratelimit,omitempty"`
    IPRatelimitSize string `yaml:"ip_ratelimit_size,omitempty" json:"ip_ratelimit_size,omitempty"`

    // TLS upstream (advanced)
    TLSCertBundle string `yaml:"tls_cert_bundle,omitempty" json:"tls_cert_bundle,omitempty"`
    TLSUpstream   bool   `yaml:"tls_upstream,omitempty" json:"tls_upstream,omitempty"`

    // Access Control (advanced)
    AccessControl []ACLEntry `yaml:"access_control,omitempty" json:"access_control,omitempty"`

    // Local Data (synced from Glory-Hole local records — not user-editable)
    LocalZones []LocalZoneEntry `yaml:"-" json:"-"` // generated, not persisted
    LocalData  []string         `yaml:"-" json:"-"` // generated, not persisted
}

type ACLEntry struct {
    Netblock string `yaml:"netblock" json:"netblock"`
    Action   string `yaml:"action" json:"action"`
}

type LocalZoneEntry struct {
    Name string `yaml:"name" json:"name"`
    Type string `yaml:"type" json:"type"`
}

type ForwardZone struct {
    Name         string   `yaml:"name" json:"name"`
    ForwardAddrs []string `yaml:"forward_addrs" json:"forward_addrs"`
    ForwardFirst bool     `yaml:"forward_first,omitempty" json:"forward_first,omitempty"`
    ForwardTLS   bool     `yaml:"forward_tls_upstream,omitempty" json:"forward_tls_upstream,omitempty"`
}

type StubZone struct {
    Name      string   `yaml:"name" json:"name"`
    StubAddrs []string `yaml:"stub_addrs" json:"stub_addrs"`
    StubPrime bool     `yaml:"stub_prime,omitempty" json:"stub_prime,omitempty"`
    StubFirst bool     `yaml:"stub_first,omitempty" json:"stub_first,omitempty"`
    StubTLS   bool     `yaml:"stub_tls_upstream,omitempty" json:"stub_tls_upstream,omitempty"`
}

type AuthZone struct {
    Name            string   `yaml:"name" json:"name"`
    Primaries       []string `yaml:"primaries,omitempty" json:"primaries,omitempty"`
    URLs            []string `yaml:"urls,omitempty" json:"urls,omitempty"`
    Zonefile        string   `yaml:"zonefile,omitempty" json:"zonefile,omitempty"`
    FallbackEnabled bool     `yaml:"fallback_enabled,omitempty" json:"fallback_enabled,omitempty"`
    ForDownstream   bool     `yaml:"for_downstream,omitempty" json:"for_downstream,omitempty"`
    ForUpstream     bool     `yaml:"for_upstream,omitempty" json:"for_upstream,omitempty"`
}

type RemoteControl struct {
    Enabled          bool   `yaml:"enabled" json:"enabled"`
    ControlInterface string `yaml:"control_interface" json:"control_interface"`
    ControlUseCert   bool   `yaml:"control_use_cert" json:"control_use_cert"`
}

type RPZConfig struct {
    Name           string `yaml:"name" json:"name"`
    URL            string `yaml:"url,omitempty" json:"url,omitempty"`
    Zonefile       string `yaml:"zonefile,omitempty" json:"zonefile,omitempty"`
    ActionOverride string `yaml:"rpz_action_override,omitempty" json:"rpz_action_override,omitempty"`
    Log            bool   `yaml:"rpz_log,omitempty" json:"rpz_log,omitempty"`
    LogName        string `yaml:"rpz_log_name,omitempty" json:"rpz_log_name,omitempty"`
}
```

#### 2.3 Defaults (`pkg/unbound/defaults.go`)

`DefaultServerConfig() *UnboundServerConfig` returns sensible defaults
matching the shipped `unbound.conf` from Phase 1:
- Cache: 64m msg, 128m rrset, 86400 max TTL
- DNSSEC: validator iterator, all harden flags on
- Performance: 2 threads, so-reuseport on
- Serve stale: on, 86400 TTL
- Remote control: enabled, unix socket, no cert

#### 2.4 Config serializer (`pkg/unbound/writer.go`)

Uses `text/template` for clean separation of Go logic and output format:

```go
func WriteConfig(cfg *UnboundServerConfig, path string) error {
    // 1. Render template to buffer
    var buf bytes.Buffer
    if err := confTemplate.Execute(&buf, cfg); err != nil {
        return fmt.Errorf("render unbound.conf: %w", err)
    }
    // 2. Atomic write: path.tmp → rename → path
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, buf.Bytes(), 0644); err != nil {
        return fmt.Errorf("write temp config: %w", err)
    }
    return os.Rename(tmp, path)
}
```

Template rules:
- Boolean fields: `yes`/`no`
- Zero-value int/string fields: omitted (Unbound uses its own defaults)
- Lists (forward-zone, stub-zone, etc.): iterated with `{{range}}`
- LocalData/LocalZones: injected at render time (not persisted in config.yml)

#### 2.5 Config validator (`pkg/unbound/validator.go`)

```go
func Validate(cfg *UnboundServerConfig, checkconfBin string) error {
    tmp, err := os.CreateTemp("", "unbound-*.conf")
    // ... write config to temp file ...
    defer os.Remove(tmp.Name())

    out, err := exec.Command(checkconfBin, tmp.Name()).CombinedOutput()
    if err != nil {
        return fmt.Errorf("unbound-checkconf: %s", strings.TrimSpace(string(out)))
    }
    return nil
}
```

#### 2.6 Persistence

The Unbound config is stored in Glory-Hole's `config.yml` under
`unbound.config`. On startup and after every API save:
1. Go struct → serialize to `unbound.conf` via template
2. Validate with `unbound-checkconf`
3. Persist to `config.yml`
4. Reload via `unbound-control reload`

```yaml
# In config.yml:
unbound:
  enabled: true
  managed: true
  config:
    server:
      msg_cache_size: "64m"
      rrset_cache_size: "128m"
      num_threads: 2
    forward_zones:
      - name: "internal.corp."
        forward_addrs: ["10.0.0.1"]
        forward_tls_upstream: true
```

---

### Phase 3: API endpoints + stats
**Goal**: REST API for Unbound config and stats. Consolidated endpoints
(fewer than the original plan — 1 server config endpoint, not 8).

#### 3.1 Middleware guard

All `/api/unbound/*` routes are wrapped in a middleware that returns `503
Service Unavailable` when `unbound.enabled` is false. This is a single
check, not per-handler.

```go
func (s *Server) unboundGuard(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if s.unboundSupervisor == nil {
            s.writeError(w, http.StatusServiceUnavailable, "Unbound resolver is not enabled")
            return
        }
        next(w, r)
    }
}
```

#### 3.2 Endpoints

```
GET  /api/unbound/status              → supervisor state + unbound-control status
GET  /api/unbound/stats               → parsed unbound-control stats_noreset (cached 5s)
GET  /api/unbound/config              → full UnboundServerConfig JSON

PUT  /api/unbound/config/server       → update server block (cache, DNSSEC, perf, etc.)

GET  /api/unbound/forward-zones       → list
POST /api/unbound/forward-zones       → add
PUT  /api/unbound/forward-zones/{name} → update
DELETE /api/unbound/forward-zones/{name} → delete

GET  /api/unbound/stub-zones          → list
POST /api/unbound/stub-zones          → add
PUT  /api/unbound/stub-zones/{name}   → update
DELETE /api/unbound/stub-zones/{name} → delete

POST /api/unbound/reload              → unbound-control reload
POST /api/unbound/flush-cache         → unbound-control flush_zone .
POST /api/unbound/flush-zone/{zone}   → unbound-control flush_zone <zone>
```

**Consolidated server config**: One `PUT /api/unbound/config/server` endpoint
accepts partial updates (only the fields present in the JSON body are applied).
This replaces the 8 separate section PUTs from the original plan. The UI sends
only the tab's fields; the handler merges them into the current config.

**Auth zones and RPZ**: Deferred to Phase 5+ as advanced features. Most home/
small-office users won't need them. Available via config.yml editing.

#### 3.3 Apply pattern (all write endpoints)

1. Parse + validate request body
2. Load current `UnboundServerConfig` from Glory-Hole's config
3. Merge changes into the relevant section
4. Validate via `unbound-checkconf` (temp file)
5. If valid: persist to `config.yml`, write `unbound.conf`, reload Unbound
6. If invalid: return 400 with error, no changes persisted
7. Return updated config section JSON

#### 3.4 Stats integration (`pkg/unbound/stats.go`)

```go
type Stats struct {
    TotalQueries     int64              `json:"total_queries"`
    CacheHits        int64              `json:"cache_hits"`
    CacheMiss        int64              `json:"cache_miss"`
    CacheHitRate     float64            `json:"cache_hit_rate"`
    AvgRecursionMs   float64            `json:"avg_recursion_ms"`
    MsgCacheCount    int64              `json:"msg_cache_count"`
    RRSetCacheCount  int64              `json:"rrset_cache_count"`
    MemTotalBytes    int64              `json:"mem_total_bytes"`
    MemCacheBytes    int64              `json:"mem_cache_bytes"`
    UptimeSeconds    int64              `json:"uptime_seconds"`
    QueryTypes       map[string]int64   `json:"query_types"`
    ResponseCodes    map[string]int64   `json:"response_codes"`
}
```

`GetStats()` runs `unbound-control stats_noreset` and parses the key=value
output. Results are cached for 5 seconds (in-memory) to avoid hammering
`unbound-control` when multiple dashboard clients poll simultaneously.

---

### Phase 4: UI — resolver pages + dashboard integration
**Goal**: 3 pages in the dashboard for Unbound management.

#### 4.1 Navigation

Add "Resolver" collapsible section to sidebar in `DashboardLayout.astro`
(hidden when Unbound is disabled — determined by a feature flag from
`GET /api/unbound/status` or `GET /api/features`):

```ts
// Added to navLinks array:
{ id: "resolver", label: "Overview", href: "/resolver", icon: "server", section: "Resolver" },
{ id: "resolver-settings", label: "Settings", href: "/resolver/settings", icon: "sliders", section: "Resolver" },
{ id: "resolver-zones", label: "Zones", href: "/resolver/zones", icon: "globe", section: "Resolver" },
```

The section is rendered client-side only when the feature flag is active.
Server-side (Astro SSG), all pages are always built — the guard is in the
React component (`if (!unboundEnabled) return <NotEnabled />`).

#### 4.2 File structure

```
src/components/resolver/
  ResolverOverviewPage.tsx   — status + stats dashboard
  ResolverSettingsPage.tsx   — tabbed settings form (essential tier only)
  ResolverZonesPage.tsx      — tabbed CRUD for forward/stub zones

src/pages/resolver/
  index.astro                → ResolverOverviewPage (client:load)
  settings.astro             → ResolverSettingsPage (client:load)
  zones.astro                → ResolverZonesPage (client:load)
```

#### 4.3 Resolver Overview page

Status cards (same `StatCard` pattern as `DashboardOverview.tsx`):
- **Status**: Running / Degraded / Failed (with error message)
- **DNSSEC**: Active / Disabled
- **Cache Hit Rate**: percentage donut
- **Uptime**: human-readable duration

Charts (Recharts, same style as main dashboard):
- **Cache efficiency**: donut (hits vs misses)
- **Memory usage**: bar chart (cache, module, total)

Action buttons:
- **Flush Cache** — `POST /api/unbound/flush-cache`
- **Reload Config** — `POST /api/unbound/reload`

#### 4.4 Resolver Settings page

Tabs (essential tier only):
- **Cache** — msg_cache_size, rrset_cache_size, max TTL, min TTL, negative TTL
- **DNSSEC** — enable/disable toggle, domain_insecure list, harden_dnssec toggle
- **Hardening** — qname_minimisation, aggressive_nsec, harden_glue, harden_below_nx
- **Serve Stale** — serve_expired toggle, TTL
- **Logging** — verbosity slider (0-5)
- **Performance** — num_threads (with note: requires Unbound restart)

Each tab submits via `PUT /api/unbound/config/server` with only its fields.
Toast on success/error.

#### 4.5 Resolver Zones page

Tabs:
- **Forward Zones** — table + add/edit dialog (same CRUD pattern as PoliciesPage)
- **Stub Zones** — table + add/edit dialog

Zone dialog fields:
- Forward: name, addresses (multi-input), TLS toggle, forward-first toggle
- Stub: name, addresses (multi-input), prime toggle, first toggle, TLS toggle

#### 4.6 Dashboard integration

Update `DashboardOverview.tsx`:
- Add conditional "Resolver" card row when Unbound is enabled
- Cards: cache hit rate, DNSSEC status, avg recursion latency
- `fetchUnboundStats()` called alongside existing `fetchStats()`
- Graceful degradation: if the endpoint returns 503, hide the row

---

### Phase 5: Feature consolidation
**Goal**: Clean up overlapping features between Glory-Hole and Unbound.

#### 5.1 Local records → Unbound local-data

When Glory-Hole local records change (API or config reload):
1. Generate `local-zone`, `local-data`, and `domain-insecure` directives
2. Inject into the `UnboundServerConfig.Server.LocalZones` / `LocalData`
   fields (these are `yaml:"-"` — generated at write time, not persisted)
3. Regenerate `unbound.conf` and reload

```
Glory-Hole A record "nas.local → 10.0.0.5"
  → local-zone: "nas.local." transparent
  → local-data: "nas.local. A 10.0.0.5"
  → domain-insecure: "nas.local."
```

#### 5.2 Conditional forwarding migration

**Manual, not automatic.** The UI shows a banner on the Forwarding page:
"Some domain-only forwarding rules can be migrated to Unbound forward-zones
for better performance and TLS support. [Migrate Now]"

Clicking "Migrate Now":
1. Shows a preview of which rules will be migrated
2. User confirms
3. API creates forward-zones in Unbound config
4. Original rules are marked `migrated_to_unbound: true` (disabled, preserved)
5. Revert available by re-enabling the original rules and deleting the
   forward-zones

This avoids surprising behavior on upgrade and gives the user control.

#### 5.3 Cache transition

When `unbound.enabled` changes:
- `true → false`: Re-enable Glory-Hole cache, restore upstream_dns_servers
- `false → true`: Disable Glory-Hole cache, clear it, point forwarder at Unbound
- Settings UI shows a notice on the Cache tab: "DNS caching is handled by the
  Unbound resolver. Configure cache settings in Resolver → Settings."

#### 5.4 Upstream DNS servers transition

When Unbound is enabled:
- Settings page DNS tab shows: "Upstream resolution is handled by Unbound.
  Configure forwarding in Resolver → Zones."
- The upstream_dns_servers input is disabled
- If Unbound is later disabled, previous upstreams are restored from config

---

### Phase 6: Non-Docker support
**Goal**: Unbound integration works outside Docker.

#### 6.1 Binary auto-detection

On startup, if `unbound.enabled: true` and `binary_path` is empty:
1. `exec.LookPath("unbound")`
2. Fallback: `/usr/sbin/unbound`, `/usr/local/sbin/unbound`
3. Same for `unbound-control` and `unbound-checkconf`
4. If not found: log warning, set `unbound.enabled: false` in effective config

#### 6.2 External mode (`managed: false`)

- Supervisor is not started
- Config writer is not used
- `unbound.conf` is not touched
- Config write endpoints return 503
- Stats endpoints work if control socket is accessible
- Glory-Hole forwards to `127.0.0.1:<listen_port>`

This mode is for users who manage Unbound via systemd or other means.

#### 6.3 SystemD template

Ship `contrib/glory-hole.service`:
```ini
[Unit]
Description=Glory-Hole DNS Server
After=network.target
Wants=unbound.service

[Service]
Type=simple
ExecStart=/usr/local/bin/glory-hole -config /etc/glory-hole/config.yml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

With `managed: false`, Unbound runs as its own unit.
With `managed: true`, Glory-Hole supervises Unbound (no separate unit needed).

#### 6.4 Documentation
- Migration guide for users of klutchell/unbound-docker
- Config reference for all Unbound settings
- Architecture diagram
- Troubleshooting (DNSSEC failures, port conflicts, permissions)

---

## Testing strategy

### Phase 1 tests

**Unit tests** (`pkg/unbound/supervisor_test.go`):
- Supervisor state machine transitions (stopped→starting→running→degraded→failed)
- Readiness gate timeout behavior
- Crash restart with backoff calculation
- Orphan detection (mock port listener)
- Shutdown signal sequence

**Integration test** (requires `unbound` binary):
- Start supervisor with default config
- Verify readiness gate completes
- Send DNS query through 127.0.0.1:5353
- Verify DNSSEC validation works (query `dnssec-failed.org` → SERVFAIL)
- Kill Unbound process, verify supervisor restarts it
- Verify health check detects failure and recovery
- Build constraint: `//go:build integration`

### Phase 2 tests

**Golden file tests** (`pkg/unbound/writer_test.go`):
- `DefaultServerConfig()` → serialize → compare to `testdata/default.conf`
- Config with forward-zones → serialize → compare to `testdata/forward.conf`
- Config with all fields populated → serialize → compare to `testdata/full.conf`
- Zero-value fields are omitted in output

**Validator test** (requires `unbound-checkconf`):
- Valid config passes
- Invalid config (bad directive) returns error with message
- Build constraint: `//go:build integration`

**Round-trip test**:
- `DefaultServerConfig()` → `WriteConfig()` → `Validate()` → no error

### Phase 3 tests

**API handler tests** (`pkg/api/handlers_unbound_test.go`):
- All endpoints return 503 when supervisor is nil
- GET /api/unbound/status returns correct state
- PUT /api/unbound/config/server with partial payload merges correctly
- Forward zone CRUD operations
- Stats caching (two rapid calls, only one `unbound-control` invocation)

### Phase 4 tests

- Astro build succeeds with new resolver pages
- TypeScript types match Go API response shapes (manual review)

---

## Priority order

1. **Phase 1** — Bundle + supervise (biggest value: DNSSEC + recursion)
2. **Phase 2** — Config model + serializer (foundation for API/UI)
3. **Phase 3** — API + stats (enables UI, dashboard integration)
4. **Phase 4** — UI pages (user-facing config management)
5. **Phase 5** — Feature consolidation (migration, overlap cleanup)
6. **Phase 6** — Non-Docker support (broader audience)

## Estimated effort

| Phase | Scope | Estimate |
|-------|-------|----------|
| 1 | Dockerfile, supervisor, entrypoint, startup wiring, fallback | 2 sessions |
| 2 | Config model (two-tier), serializer (template), validator, defaults | 1 session |
| 3 | ~15 API endpoints, stats parser, middleware guard, handlers | 1-2 sessions |
| 4 | 3 React components, 3 Astro pages, API client, dashboard integration | 2 sessions |
| 5 | Local record sync, manual forwarding migration, cache/upstream transition | 1 session |
| 6 | Auto-detection, external mode, systemd template, docs | 1 session |
