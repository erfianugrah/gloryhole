# Glory-Hole Design Document

## Overview

This document outlines the design and implementation plan for advanced features in Glory-Hole DNS server, including client management, group management, rate-limiting, conditional forwarding, and enhanced local DNS records.

## Table of Contents

1. [Client Management](#client-management)
2. [Group Management](#group-management)
3. [Rate Limiting](#rate-limiting)
4. [Conditional Forwarding](#conditional-forwarding)
5. [Local DNS Records](#local-dns-records)
6. [CNAME Records](#cname-records)
7. [DNS Request Processing Pipeline](#dns-request-processing-pipeline)
8. [Configuration Schema](#configuration-schema)
9. [Storage Schema](#storage-schema)
10. [API Endpoints](#api-endpoints)

---

## Client Management

### Purpose
Track and manage individual DNS clients with custom settings, policies, and statistics.

### Data Structure

```go
type Client struct {
    ID          string            // Unique identifier (UUID or IP-based)
    IP          net.IP            // Client IP address
    MAC         string            // Optional MAC address for DHCP correlation
    Name        string            // Human-readable name
    Groups      []string          // Group memberships
    Tags        []string          // Custom tags for organization
    Settings    *ClientSettings   // Client-specific settings
    Stats       *ClientStats      // Query statistics
    Created     time.Time         // First seen timestamp
    LastSeen    time.Time         // Last query timestamp
    Enabled     bool              // Whether client is active
}

type ClientSettings struct {
    RateLimit        *RateLimit          // Per-client rate limiting
    CustomBlocklist  []string            // Additional blocked domains
    CustomWhitelist  []string            // Additional allowed domains
    UseGroups        bool                // Inherit group settings
    LogQueries       bool                // Log this client's queries
    CustomUpstreams  []string            // Client-specific DNS servers
}

type ClientStats struct {
    TotalQueries    uint64
    BlockedQueries  uint64
    CachedQueries   uint64
    LastQueryTime   time.Time
    TopDomains      map[string]uint64  // Domain → count
}
```

### Implementation Details

**Storage:**
- SQLite table for client definitions
- In-memory cache for fast lookups during DNS queries
- Periodic sync between memory and disk

**Identification:**
- Primary: Source IP address from DNS request
- Secondary: DHCP lease correlation (if enabled)
- Fallback: Create dynamic client entry for unknown IPs

**Auto-Discovery:**
- Automatically create client entries on first query
- Mark as "unmanaged" until user assigns name/groups
- Configurable auto-discovery mode (enabled/disabled/existing-only)

**Location:** `pkg/client/manager.go`

### Configuration

```yaml
client_management:
  enabled: true
  auto_discovery: true  # Create entries for new clients
  default_group: "default"

clients:
  - name: "John's Laptop"
    ip: "192.168.1.100"
    groups: ["family", "adults"]
    rate_limit:
      requests_per_second: 50
      burst: 100
    log_queries: true

  - name: "Kids iPad"
    ip: "192.168.1.150"
    groups: ["family", "kids"]
    custom_blocklist:
      - "*.tiktok.com"
      - "*.snapchat.com"
    rate_limit:
      requests_per_second: 30
```

---

## Group Management

### Purpose
Organize clients into logical groups with shared policies, enabling bulk management and hierarchical rule application.

### Data Structure

```go
type Group struct {
    Name            string              // Unique group name
    Description     string              // Human-readable description
    Priority        int                 // Rule evaluation priority (higher = first)

    // Members
    ClientIDs       []string            // Explicit client memberships
    IPRanges        []IPRange           // CIDR ranges for auto-membership

    // DNS Settings
    Blocklists      []string            // Group-specific blocklists
    Whitelist       []string            // Group-specific whitelist
    CustomOverrides map[string]net.IP   // Group-specific DNS overrides

    // Forwarding
    Upstreams       []string            // Group-specific DNS servers

    // Policies
    RateLimit       *RateLimit          // Group-level rate limiting
    Schedule        *Schedule           // Time-based rule activation
    PolicyRules     []*PolicyRule       // Custom policy rules

    // Options
    Enabled         bool                // Whether group is active
    InheritGlobal   bool                // Inherit global blocklist/whitelist
    LogQueries      bool                // Log queries from this group
}

type IPRange struct {
    CIDR        string    // e.g., "192.168.1.0/24"
    Description string
}

type Schedule struct {
    Enabled     bool
    TimeRanges  []TimeRange
    Weekdays    []time.Weekday  // nil = all days
    DateRanges  []DateRange
}

type TimeRange struct {
    Start   string  // "09:00"
    End     string  // "17:00"
}

type DateRange struct {
    Start   time.Time
    End     time.Time
}
```

### Use Cases

**Family Groups:**
```yaml
groups:
  - name: "kids"
    description: "Children's devices"
    blocklists:
      - "https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/fakenews-gambling-porn/hosts"
    schedule:
      enabled: true
      time_ranges:
        - start: "07:00"
          end: "21:00"
      weekdays: [1,2,3,4,5]  # Monday-Friday
    rate_limit:
      requests_per_second: 30
      burst: 50
    policy_rules:
      - name: "Block social media after 8pm"
        logic: "Hour >= 20 && Domain matches '.*(tiktok|instagram|snapchat)\\.com'"
        action: "BLOCK"

  - name: "adults"
    description: "Adult family members"
    inherit_global: true
    rate_limit:
      requests_per_second: 100
```

**Network Segments:**
```yaml
groups:
  - name: "iot"
    description: "IoT devices"
    ip_ranges:
      - cidr: "192.168.10.0/24"
        description: "IoT VLAN"
    blocklists:
      - "https://v.firebog.net/hosts/Prigent-Malware.txt"
    whitelist:
      - "*.amazonaws.com"  # Allow AWS IoT
      - "*.google.com"     # Allow Google Home
    rate_limit:
      requests_per_second: 10
      burst: 20

  - name: "guests"
    description: "Guest WiFi"
    ip_ranges:
      - cidr: "192.168.20.0/24"
        description: "Guest network"
    rate_limit:
      requests_per_second: 20
      burst: 40
    log_queries: false
```

**Work From Home:**
```yaml
groups:
  - name: "work"
    description: "Work devices during business hours"
    schedule:
      enabled: true
      time_ranges:
        - start: "09:00"
          end: "17:00"
      weekdays: [1,2,3,4,5]
    upstreams:
      - "10.0.0.1:53"  # Corporate DNS
    whitelist:
      - "*.company.com"
    custom_overrides:
      intranet.company.com: "10.0.0.100"
```

### Rule Evaluation Order

1. **Client-specific rules** (highest priority)
2. **Group rules** (by priority value, highest first)
3. **Multiple group membership**: OR logic for whitelists, AND logic for blocks
4. **Global rules** (lowest priority)

### Implementation Details

**Location:** `pkg/group/manager.go`

**Key Operations:**
- `GetGroupsForClient(clientIP) []Group` - Resolve client's groups
- `EvaluateGroups(client, domain) (action, group)` - Apply group rules
- `ReloadGroups()` - Hot reload group configuration

---

## Rate Limiting

### Purpose
Protect against DNS abuse, amplification attacks, and excessive queries from misbehaving clients.

### Data Structure

```go
type RateLimit struct {
    // Token bucket parameters
    RequestsPerSecond int           // Sustained rate
    BurstSize         int           // Maximum burst

    // Sliding window parameters (alternative)
    WindowSize        time.Duration // e.g., 1 minute
    MaxRequests       int           // Max requests per window

    // Actions
    OnExceed          string        // "drop", "delay", "nxdomain"
    LogViolations     bool
    BlockDuration     time.Duration // Temporary block time
}

type RateLimiter struct {
    limiters    sync.Map           // clientIP → *rate.Limiter
    violations  sync.Map           // clientIP → []time.Time
    blocked     sync.Map           // clientIP → time.Time (block expiry)

    globalLimit *RateLimit
    mu          sync.RWMutex
}
```

### Implementation Strategies

**Token Bucket (Recommended):**
- Uses `golang.org/x/time/rate` package
- Allows burst traffic while maintaining average rate
- Memory efficient
- Configurable refill rate and bucket size

```go
func (rl *RateLimiter) Allow(clientIP string) bool {
    limiter := rl.getLimiter(clientIP)
    return limiter.Allow()
}

func (rl *RateLimiter) getLimiter(clientIP string) *rate.Limiter {
    if limiter, exists := rl.limiters.Load(clientIP); exists {
        return limiter.(*rate.Limiter)
    }

    limiter := rate.NewLimiter(
        rate.Limit(rl.globalLimit.RequestsPerSecond),
        rl.globalLimit.BurstSize,
    )
    rl.limiters.Store(clientIP, limiter)
    return limiter
}
```

**Sliding Window:**
- Track timestamps of recent requests
- More accurate but higher memory usage
- Better for strict enforcement

### Configuration

```yaml
rate_limiting:
  enabled: true

  # Global default
  global:
    requests_per_second: 50
    burst: 100
    on_exceed: "drop"  # drop, delay, nxdomain
    log_violations: true
    block_duration: "5m"

  # Per-client limits in client definitions
  # Per-group limits in group definitions

  # Cleanup
  cleanup_interval: "10m"  # Remove inactive limiters
  max_tracked_clients: 10000
```

### Actions on Rate Limit Exceeded

**Drop:**
- Silently drop the request
- No response sent
- Lowest overhead

**Delay:**
- Artificial delay before processing
- Throttles client naturally
- Higher overhead

**NXDOMAIN:**
- Return "domain not found"
- Client receives response
- May cause client retries

**Temporary Block:**
- Block client for configured duration
- Logged as violation
- Automatic unblock after timeout

### Monitoring

```go
type RateLimitStats struct {
    ClientIP            string
    TotalRequests       uint64
    AllowedRequests     uint64
    DroppedRequests     uint64
    CurrentRate         float64
    LastViolation       time.Time
    ViolationCount      int
}
```

### Location
`pkg/ratelimit/limiter.go`

---

## Conditional Forwarding

### Purpose
Forward specific domains or DNS zones to designated upstream DNS servers, enabling split-DNS configurations for hybrid networks.

### Data Structure

```go
type ConditionalForwarder struct {
    // Domain matching
    Domain          string        // e.g., "corp.internal", "10.in-addr.arpa"
    MatchType       MatchType     // exact, suffix, prefix, regex

    // Forwarding targets
    Upstreams       []string      // Specific DNS servers for this domain
    Fallback        bool          // Fall back to global upstreams on failure

    // Options
    Priority        int           // Higher priority checked first
    Enabled         bool

    // Performance
    CacheTTL        time.Duration // Override TTL for cached responses
}

type MatchType string

const (
    MatchExact  MatchType = "exact"   // Exact domain match
    MatchSuffix MatchType = "suffix"  // domain ends with
    MatchPrefix MatchType = "prefix"  // domain starts with
    MatchRegex  MatchType = "regex"   // Regex pattern
)

type ConditionalForwarderManager struct {
    forwarders  []*ConditionalForwarder  // Sorted by priority
    mu          sync.RWMutex
}
```

### Use Cases

**Split-DNS (Corporate + Home):**
```yaml
conditional_forwarders:
  - domain: "corp.company.com"
    match_type: "suffix"
    upstreams:
      - "10.0.0.1:53"
      - "10.0.0.2:53"
    fallback: false
    priority: 100

  - domain: "company.com"
    match_type: "suffix"
    upstreams:
      - "10.0.0.1:53"
    fallback: true
    priority: 90
```

**Reverse DNS (PTR Records):**
```yaml
conditional_forwarders:
  - domain: "1.168.192.in-addr.arpa"
    match_type: "suffix"
    upstreams:
      - "192.168.1.1:53"
    priority: 100

  - domain: "10.in-addr.arpa"
    match_type: "suffix"
    upstreams:
      - "10.0.0.1:53"
    priority: 100
```

**Local TLDs:**
```yaml
conditional_forwarders:
  - domain: "local"
    match_type: "suffix"
    upstreams:
      - "192.168.1.1:53"  # Router's DNS
    fallback: false
    priority: 100

  - domain: "home"
    match_type: "suffix"
    upstreams:
      - "192.168.1.1:53"
    fallback: false
    priority: 100
```

**Development Environments:**
```yaml
conditional_forwarders:
  - domain: "*.dev.local"
    match_type: "suffix"
    upstreams:
      - "192.168.1.50:53"  # Dev server DNS
    cache_ttl: "30s"
    priority: 100
```

### Implementation Details

**Matching Algorithm:**
```go
func (cfm *ConditionalForwarderManager) FindForwarder(domain string) *ConditionalForwarder {
    cfm.mu.RLock()
    defer cfm.mu.RUnlock()

    // Iterate by priority (pre-sorted)
    for _, fwd := range cfm.forwarders {
        if !fwd.Enabled {
            continue
        }

        switch fwd.MatchType {
        case MatchExact:
            if domain == fwd.Domain {
                return fwd
            }
        case MatchSuffix:
            if strings.HasSuffix(domain, fwd.Domain) {
                return fwd
            }
        case MatchPrefix:
            if strings.HasPrefix(domain, fwd.Domain) {
                return fwd
            }
        case MatchRegex:
            if matched, _ := regexp.MatchString(fwd.Domain, domain); matched {
                return fwd
            }
        }
    }

    return nil
}
```

**Forwarding Logic:**
1. Check if domain matches any conditional forwarder
2. If match found, query specified upstreams
3. On failure:
   - If fallback enabled: try global upstreams
   - If fallback disabled: return SERVFAIL
4. If no match: proceed with global upstreams

**Validation:**
- Ensure no circular forwarding
- Validate upstream DNS server reachability
- Check for conflicting rules (warn if multiple matches)

### Performance Considerations

- Pre-compile regex patterns during configuration load
- Cache forwarder lookups (domain → forwarder)
- Sort forwarders by priority once during load
- Use suffix tree for efficient suffix matching

### Location
`pkg/forwarder/conditional.go`

---

## Local DNS Records

### Purpose
Serve authoritative DNS responses for local network hosts without querying upstream servers.

### Current Implementation
Basic map-based storage in `pkg/dns/server.go:17`:
```go
Overrides map[string]net.IP
```

### Enhanced Design

```go
type LocalRecord struct {
    Domain      string
    Type        RecordType      // A, AAAA, CNAME, MX, TXT, SRV, PTR
    TTL         uint32          // Time to live
    Priority    uint16          // For MX, SRV records
    Weight      uint16          // For SRV records
    Port        uint16          // For SRV records
    Target      string          // CNAME target, MX host, TXT data
    IPs         []net.IP        // Multiple IPs for A/AAAA
    Wildcard    bool            // Support *.domain.local
    Enabled     bool
}

type RecordType string

const (
    RecordTypeA     RecordType = "A"
    RecordTypeAAAA  RecordType = "AAAA"
    RecordTypeCNAME RecordType = "CNAME"
    RecordTypeMX    RecordType = "MX"
    RecordTypeTXT   RecordType = "TXT"
    RecordTypeSRV   RecordType = "SRV"
    RecordTypePTR   RecordType = "PTR"
)

type LocalRecordManager struct {
    records     map[string][]*LocalRecord  // domain → records
    wildcards   []*LocalRecord             // Wildcard records
    mu          sync.RWMutex
}
```

### Configuration

```yaml
local_records:
  # Simple A records
  - domain: "nas.local"
    type: "A"
    ip: "192.168.1.100"
    ttl: 300

  # Multiple IPs (round-robin)
  - domain: "homelab.local"
    type: "A"
    ips:
      - "192.168.1.10"
      - "192.168.1.11"
      - "192.168.1.12"
    ttl: 300

  # IPv6
  - domain: "nas.local"
    type: "AAAA"
    ip: "fe80::1"
    ttl: 300

  # CNAME
  - domain: "storage.local"
    type: "CNAME"
    target: "nas.local."
    ttl: 300

  # Wildcard
  - domain: "*.dev.local"
    type: "A"
    ip: "192.168.1.50"
    wildcard: true
    ttl: 60

  # MX Record
  - domain: "mail.local"
    type: "MX"
    priority: 10
    target: "mailserver.local."
    ttl: 3600

  # TXT Record (SPF, DKIM, etc.)
  - domain: "local"
    type: "TXT"
    target: "v=spf1 mx -all"
    ttl: 3600

  # SRV Record (Service discovery)
  - domain: "_http._tcp.local"
    type: "SRV"
    priority: 0
    weight: 5
    port: 80
    target: "webserver.local."
    ttl: 300

  # PTR Record (Reverse DNS)
  - domain: "100.1.168.192.in-addr.arpa"
    type: "PTR"
    target: "nas.local."
    ttl: 300
```

### Advanced Features

**Dynamic DNS (DDNS):**
```go
type DDNSUpdate struct {
    Domain      string
    IP          net.IP
    TTL         uint32
    Timestamp   time.Time
    ClientID    string
}

// API endpoint: POST /api/ddns/update
func (lrm *LocalRecordManager) UpdateDynamic(domain string, ip net.IP) error {
    // Validate authorization
    // Update or create record
    // Log update
    // Notify listeners
}
```

**Record Validation:**
- CNAME cannot coexist with other record types for same name
- Validate IP addresses (IPv4/IPv6)
- Check for circular CNAME references
- Validate MX/SRV target domains exist
- Ensure PTR records match forward records

**Wildcard Matching:**
```go
func (lrm *LocalRecordManager) FindRecords(domain string, qtype RecordType) []*LocalRecord {
    // 1. Check exact match
    if records, exists := lrm.records[domain]; exists {
        return filterByType(records, qtype)
    }

    // 2. Check wildcards
    for _, wildcard := range lrm.wildcards {
        if matchesWildcard(domain, wildcard.Domain) {
            return []*LocalRecord{wildcard}
        }
    }

    return nil
}

func matchesWildcard(domain, pattern string) bool {
    // *.example.com matches foo.example.com but not example.com
    // *.example.com matches bar.baz.example.com
    pattern = strings.TrimPrefix(pattern, "*.")
    return strings.HasSuffix(domain, "." + pattern)
}
```

### Location
`pkg/localrecords/manager.go`

---

## CNAME Records

### Current Implementation
Basic string map in `pkg/dns/server.go:19`:
```go
CNAMEOverrides map[string]string
```

### Enhanced Design

```go
type CNAMERecord struct {
    Source      string        // Query domain
    Target      string        // Target domain (must end with .)
    TTL         uint32
    Enabled     bool

    // Validation
    ChainDepth  int           // Track CNAME chain length
    Validated   bool          // Whether target resolution was tested
}

type CNAMEManager struct {
    records     map[string]*CNAMERecord
    chains      map[string][]string      // Detect circular refs
    mu          sync.RWMutex
}
```

### Features

**CNAME Chain Resolution:**
```go
func (cm *CNAMEManager) ResolveCNAMEChain(domain string, maxDepth int) ([]string, error) {
    chain := []string{domain}
    visited := make(map[string]bool)

    current := domain
    for depth := 0; depth < maxDepth; depth++ {
        if visited[current] {
            return nil, fmt.Errorf("circular CNAME reference detected: %v", chain)
        }
        visited[current] = true

        cname, exists := cm.records[current]
        if !exists || !cname.Enabled {
            break
        }

        current = cname.Target
        chain = append(chain, current)
    }

    if len(chain) >= maxDepth {
        return nil, fmt.Errorf("CNAME chain too deep: %v", chain)
    }

    return chain, nil
}
```

**Validation:**
```go
func (cm *CNAMEManager) Validate(record *CNAMERecord) error {
    // 1. Target must end with dot
    if !strings.HasSuffix(record.Target, ".") {
        return fmt.Errorf("CNAME target must be FQDN ending with dot")
    }

    // 2. Check for circular reference
    chain, err := cm.ResolveCNAMEChain(record.Source, 10)
    if err != nil {
        return err
    }

    // 3. Check if source has other records (DNS spec violation)
    if cm.hasOtherRecords(record.Source) {
        return fmt.Errorf("CNAME cannot coexist with other records")
    }

    return nil
}
```

**Wildcard CNAMEs:**
```yaml
cname_records:
  - source: "*.service.local"
    target: "loadbalancer.local."
    ttl: 300
    wildcard: true
```

### DNS Response Construction

```go
func (h *Handler) buildCNAMEResponse(req *dns.Msg, chain []string) *dns.Msg {
    resp := new(dns.Msg)
    resp.SetReply(req)
    resp.Authoritative = true

    // Add CNAME records for chain
    for i := 0; i < len(chain)-1; i++ {
        cname := &dns.CNAME{
            Hdr: dns.RR_Header{
                Name:   dns.Fqdn(chain[i]),
                Rrtype: dns.TypeCNAME,
                Class:  dns.ClassINET,
                Ttl:    h.cnameManager.records[chain[i]].TTL,
            },
            Target: dns.Fqdn(chain[i+1]),
        }
        resp.Answer = append(resp.Answer, cname)
    }

    // Resolve final target (if local record)
    final := chain[len(chain)-1]
    if localRecords := h.localRecordManager.FindRecords(final, req.Question[0].Qtype); len(localRecords) > 0 {
        for _, record := range localRecords {
            rr := buildRR(record)
            resp.Answer = append(resp.Answer, rr)
        }
    } else {
        // Query upstream for final target
        // Add to Additional section
    }

    return resp
}
```

### Configuration

```yaml
cname_records:
  - source: "www.local"
    target: "webserver.local."
    ttl: 300

  - source: "mail.local"
    target: "mailserver.company.com."
    ttl: 3600

  - source: "*.cdn.local"
    target: "cdn-origin.local."
    ttl: 60
    wildcard: true
```

### Location
`pkg/cname/manager.go`

---

## DNS Request Processing Pipeline

### Complete Flow

```
┌─────────────────────────────────────────────────┐
│  1. Receive DNS Request                          │
│     - Extract client IP, domain, query type      │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│  2. Client Identification                        │
│     - Lookup/create client entry                 │
│     - Load client settings                       │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│  3. Rate Limiting                                │
│     - Check client-specific limits               │
│     - Check group limits                         │
│     - Check global limits                        │
│     - Action: DROP / DELAY / CONTINUE            │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│  4. Group Resolution                             │
│     - Resolve client's group memberships         │
│     - Load group policies                        │
│     - Merge client + group settings              │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│  5. Cache Lookup                                 │
│     - Check cache for domain                     │
│     - Validate TTL                               │
│     - Return if hit (log + stats)                │
└────────────────┬────────────────────────────────┘
                 │ (cache miss)
                 v
┌─────────────────────────────────────────────────┐
│  6. Policy Engine Evaluation                     │
│     - Evaluate custom rules with context:        │
│       * ClientIP, Domain, Hour, Weekday          │
│       * Groups, Tags                             │
│     - Action: ALLOW / BLOCK / CUSTOM             │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│  7. Local Records Check                          │
│     - Check for exact match                      │
│     - Check wildcard patterns                    │
│     - If found: build authoritative response     │
└────────────────┬────────────────────────────────┘
                 │ (not found)
                 v
┌─────────────────────────────────────────────────┐
│  8. CNAME Records Check                          │
│     - Check for CNAME mapping                    │
│     - Resolve CNAME chain                        │
│     - If found: build CNAME response             │
└────────────────┬────────────────────────────────┘
                 │ (not found)
                 v
┌─────────────────────────────────────────────────┐
│  9. Allowlist Check                              │
│     - Client allowlist                           │
│     - Group allowlist                            │
│     - Global allowlist                           │
│     - If matched: skip blocking, forward         │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│ 10. Blocklist Check                              │
│     - Client blocklist                           │
│     - Group blocklist                            │
│     - Global blocklist                           │
│     - If matched: return blocked response        │
└────────────────┬────────────────────────────────┘
                 │ (not blocked)
                 v
┌─────────────────────────────────────────────────┐
│ 11. Conditional Forwarding                       │
│     - Check domain against forwarders            │
│     - If matched: query specific upstreams       │
│     - Fallback to global if enabled              │
└────────────────┬────────────────────────────────┘
                 │ (no match)
                 v
┌─────────────────────────────────────────────────┐
│ 12. Upstream Forwarding                          │
│     - Select upstream (group/client/global)      │
│     - Forward query                              │
│     - Receive response                           │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│ 13. Post-Processing                              │
│     - Cache response (if cacheable)              │
│     - Log query (if enabled)                     │
│     - Update statistics                          │
│     - Return response to client                  │
└─────────────────────────────────────────────────┘
```

### Context Object

```go
type QueryContext struct {
    // Request
    Request     *dns.Msg
    Domain      string
    QueryType   uint16
    ClientIP    net.IP
    Timestamp   time.Time

    // Client/Group
    Client      *Client
    Groups      []*Group

    // Decision tracking
    Decision    Decision
    Reason      string
    MatchedRule *Rule
    Cached      bool

    // Performance
    StartTime   time.Time
    Duration    time.Duration

    // Response
    Response    *dns.Msg
    Status      ResponseStatus
}

type Decision string

const (
    DecisionAllow    Decision = "ALLOW"
    DecisionBlock    Decision = "BLOCK"
    DecisionCache    Decision = "CACHE"
    DecisionForward  Decision = "FORWARD"
    DecisionLocal    Decision = "LOCAL"
    DecisionCNAME    Decision = "CNAME"
    DecisionDrop     Decision = "DROP"     // Rate limited
)

type ResponseStatus string

const (
    StatusSuccess     ResponseStatus = "SUCCESS"
    StatusBlocked     ResponseStatus = "BLOCKED"
    StatusCached      ResponseStatus = "CACHED"
    StatusNXDOMAIN    ResponseStatus = "NXDOMAIN"
    StatusSERVFAIL    ResponseStatus = "SERVFAIL"
    StatusRateLimited ResponseStatus = "RATE_LIMITED"
)
```

### Implementation in ServeDNS

```go
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
    qctx := &QueryContext{
        Request:   r,
        ClientIP:  getClientIP(w),
        Timestamp: time.Now(),
        StartTime: time.Now(),
    }

    defer func() {
        qctx.Duration = time.Since(qctx.StartTime)
        h.logQuery(qctx)
        h.updateStats(qctx)
    }()

    // Extract query details
    if len(r.Question) == 0 {
        return
    }
    qctx.Domain = strings.TrimSuffix(r.Question[0].Name, ".")
    qctx.QueryType = r.Question[0].Qtype

    // 1-2. Client identification
    qctx.Client = h.clientManager.GetOrCreateClient(qctx.ClientIP)

    // 3. Rate limiting
    if !h.rateLimiter.Allow(qctx) {
        qctx.Decision = DecisionDrop
        qctx.Status = StatusRateLimited
        // Drop or send rate limit response
        return
    }

    // 4. Group resolution
    qctx.Groups = h.groupManager.GetGroupsForClient(qctx.Client)

    // 5. Cache lookup
    if cached := h.cache.Get(qctx.Domain, qctx.QueryType); cached != nil {
        qctx.Response = cached
        qctx.Decision = DecisionCache
        qctx.Status = StatusCached
        qctx.Cached = true
        cached.WriteTo(w)
        return
    }

    // 6. Policy engine
    if blocked, rule := h.policyEngine.Evaluate(qctx); blocked {
        qctx.Decision = DecisionBlock
        qctx.Status = StatusBlocked
        qctx.MatchedRule = rule
        qctx.Response = h.buildBlockedResponse(r)
        qctx.Response.WriteTo(w)
        return
    }

    // 7. Local records
    if localRecords := h.localRecordManager.FindRecords(qctx.Domain, qctx.QueryType); len(localRecords) > 0 {
        qctx.Decision = DecisionLocal
        qctx.Status = StatusSuccess
        qctx.Response = h.buildLocalResponse(r, localRecords)
        h.cache.Set(qctx.Domain, qctx.QueryType, qctx.Response)
        qctx.Response.WriteTo(w)
        return
    }

    // 8. CNAME records
    if chain, err := h.cnameManager.ResolveCNAMEChain(qctx.Domain, 10); err == nil && len(chain) > 1 {
        qctx.Decision = DecisionCNAME
        qctx.Status = StatusSuccess
        qctx.Response = h.buildCNAMEResponse(r, chain)
        h.cache.Set(qctx.Domain, qctx.QueryType, qctx.Response)
        qctx.Response.WriteTo(w)
        return
    }

    // 9. Allowlist check
    if h.isAllowlisted(qctx) {
        // Skip blocklist, proceed to forwarding
        goto forward
    }

    // 10. Blocklist check
    if h.isBlocklisted(qctx) {
        qctx.Decision = DecisionBlock
        qctx.Status = StatusBlocked
        qctx.Response = h.buildBlockedResponse(r)
        qctx.Response.WriteTo(w)
        return
    }

forward:
    // 11-12. Conditional or upstream forwarding
    upstreams := h.selectUpstreams(qctx)
    resp, err := h.forwardQuery(r, upstreams)
    if err != nil {
        qctx.Status = StatusSERVFAIL
        h.buildErrorResponse(r).WriteTo(w)
        return
    }

    qctx.Response = resp
    qctx.Decision = DecisionForward
    qctx.Status = StatusSuccess

    // 13. Cache and return
    h.cache.Set(qctx.Domain, qctx.QueryType, resp)
    resp.WriteTo(w)
}
```

---

## Configuration Schema

### Complete YAML Structure

```yaml
# Server settings
server:
  listen_address: ":53"
  tcp_enabled: true
  udp_enabled: true
  web_ui_address: ":8080"

# Upstream DNS servers (global default)
upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

# Update settings
update_interval: "24h"
auto_update_blocklists: true

# Storage
storage:
  database_path: "./gloryhole.db"
  log_queries: true
  log_retention_days: 30
  buffer_size: 1000

# Cache settings
cache:
  enabled: true
  max_entries: 10000
  min_ttl: 60
  max_ttl: 86400
  negative_ttl: 300

# Rate limiting
rate_limiting:
  enabled: true
  global:
    requests_per_second: 50
    burst: 100
    on_exceed: "drop"
    log_violations: true
    block_duration: "5m"
  cleanup_interval: "10m"
  max_tracked_clients: 10000

# Client management
client_management:
  enabled: true
  auto_discovery: true
  default_group: "default"

clients:
  - name: "John's Laptop"
    ip: "192.168.1.100"
    mac: "aa:bb:cc:dd:ee:ff"
    groups: ["family", "adults"]
    tags: ["laptop", "trusted"]
    settings:
      rate_limit:
        requests_per_second: 100
        burst: 200
      log_queries: true
      custom_upstreams:
        - "1.1.1.1:53"

  - name: "Kids iPad"
    ip: "192.168.1.150"
    groups: ["family", "kids"]
    settings:
      rate_limit:
        requests_per_second: 30
        burst: 50
      custom_blocklist:
        - "*.tiktok.com"
        - "*.snapchat.com"

# Group management
groups:
  - name: "kids"
    description: "Children's devices"
    priority: 100
    blocklists:
      - "https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/fakenews-gambling-porn/hosts"
    schedule:
      enabled: true
      time_ranges:
        - start: "07:00"
          end: "21:00"
      weekdays: [1,2,3,4,5]  # Monday-Friday
    rate_limit:
      requests_per_second: 30
      burst: 50
    policy_rules:
      - name: "Block social media after 8pm"
        logic: "Hour >= 20 && Domain matches '.*(tiktok|instagram|snapchat)\\.com'"
        action: "BLOCK"

  - name: "iot"
    description: "IoT devices"
    priority: 50
    ip_ranges:
      - cidr: "192.168.10.0/24"
        description: "IoT VLAN"
    rate_limit:
      requests_per_second: 10
      burst: 20
    whitelist:
      - "*.amazonaws.com"
      - "*.google.com"

# Global blocklists
blocklists:
  - "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"
  - "https://v.firebog.net/hosts/AdguardDNS.txt"

# Global whitelist
whitelist:
  - "whitelisted-domain.com"
  - "*.important-service.com"

# Local DNS records
local_records:
  - domain: "nas.local"
    type: "A"
    ip: "192.168.1.100"
    ttl: 300

  - domain: "nas.local"
    type: "AAAA"
    ip: "fe80::1"
    ttl: 300

  - domain: "*.dev.local"
    type: "A"
    ip: "192.168.1.50"
    wildcard: true
    ttl: 60

# CNAME records
cname_records:
  - source: "www.local"
    target: "nas.local."
    ttl: 300

  - source: "storage.local"
    target: "nas.local."
    ttl: 300

# Conditional forwarding
conditional_forwarders:
  - domain: "corp.company.com"
    match_type: "suffix"
    upstreams:
      - "10.0.0.1:53"
      - "10.0.0.2:53"
    fallback: false
    priority: 100

  - domain: "1.168.192.in-addr.arpa"
    match_type: "suffix"
    upstreams:
      - "192.168.1.1:53"
    priority: 100

# Policy rules (global)
rules:
  - name: "Block ads"
    logic: "Domain matches '.*\\.(ads|doubleclick|adservice)\\..*'"
    action: "BLOCK"

  - name: "Allow work domains"
    logic: "Domain endsWith '.company.com'"
    action: "ALLOW"
```

---

## Storage Schema

### SQLite Database Schema

```sql
-- Clients table
CREATE TABLE clients (
    id TEXT PRIMARY KEY,
    ip TEXT NOT NULL UNIQUE,
    mac TEXT,
    name TEXT NOT NULL,
    groups TEXT,  -- JSON array
    tags TEXT,    -- JSON array
    settings TEXT,  -- JSON object
    created_at INTEGER NOT NULL,
    last_seen INTEGER NOT NULL,
    enabled INTEGER DEFAULT 1
);

CREATE INDEX idx_clients_ip ON clients(ip);
CREATE INDEX idx_clients_last_seen ON clients(last_seen);

-- Groups table
CREATE TABLE groups (
    name TEXT PRIMARY KEY,
    description TEXT,
    priority INTEGER DEFAULT 0,
    config TEXT NOT NULL,  -- JSON object with all group settings
    enabled INTEGER DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Query logs
CREATE TABLE query_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,
    client_id TEXT,
    client_ip TEXT NOT NULL,
    domain TEXT NOT NULL,
    query_type INTEGER NOT NULL,
    decision TEXT NOT NULL,
    status TEXT NOT NULL,
    cached INTEGER DEFAULT 0,
    duration_ms INTEGER,
    matched_rule TEXT,
    FOREIGN KEY (client_id) REFERENCES clients(id)
);

CREATE INDEX idx_query_logs_timestamp ON query_logs(timestamp);
CREATE INDEX idx_query_logs_client_ip ON query_logs(client_ip);
CREATE INDEX idx_query_logs_domain ON query_logs(domain);
CREATE INDEX idx_query_logs_decision ON query_logs(decision);

-- Query statistics (aggregated)
CREATE TABLE query_stats (
    date TEXT NOT NULL,  -- YYYY-MM-DD
    hour INTEGER NOT NULL,  -- 0-23
    client_id TEXT,
    domain TEXT,
    total_queries INTEGER DEFAULT 0,
    blocked_queries INTEGER DEFAULT 0,
    cached_queries INTEGER DEFAULT 0,
    PRIMARY KEY (date, hour, client_id, domain),
    FOREIGN KEY (client_id) REFERENCES clients(id)
);

CREATE INDEX idx_query_stats_date ON query_stats(date);
CREATE INDEX idx_query_stats_client ON query_stats(client_id);

-- Top domains cache
CREATE TABLE top_domains (
    domain TEXT PRIMARY KEY,
    query_count INTEGER DEFAULT 0,
    last_updated INTEGER NOT NULL
);

-- Rate limit violations
CREATE TABLE rate_limit_violations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,
    client_id TEXT,
    client_ip TEXT NOT NULL,
    violation_type TEXT NOT NULL,  -- "client", "group", "global"
    requests_attempted INTEGER,
    FOREIGN KEY (client_id) REFERENCES clients(id)
);

CREATE INDEX idx_rate_violations_timestamp ON rate_limit_violations(timestamp);
CREATE INDEX idx_rate_violations_client ON rate_limit_violations(client_ip);

-- Local records (for DDNS)
CREATE TABLE local_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT NOT NULL,
    type TEXT NOT NULL,
    value TEXT NOT NULL,  -- JSON: IP, CNAME target, etc.
    ttl INTEGER DEFAULT 300,
    priority INTEGER DEFAULT 0,
    wildcard INTEGER DEFAULT 0,
    enabled INTEGER DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX idx_local_records_domain ON local_records(domain);
CREATE INDEX idx_local_records_type ON local_records(type);

-- Blocklist entries
CREATE TABLE blocklist_entries (
    domain TEXT PRIMARY KEY,
    source TEXT,  -- Which blocklist it came from
    added_at INTEGER NOT NULL
);

CREATE INDEX idx_blocklist_source ON blocklist_entries(source);

-- Whitelist entries
CREATE TABLE whitelist_entries (
    domain TEXT PRIMARY KEY,
    reason TEXT,
    added_at INTEGER NOT NULL
);

-- Configuration snapshots
CREATE TABLE config_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,
    config TEXT NOT NULL,  -- Full YAML config
    changed_by TEXT,
    change_reason TEXT
);
```

---

## API Endpoints

### Statistics & Monitoring

```
GET /api/stats
Response: {
    "total_queries": 1234567,
    "blocked_queries": 123456,
    "cached_queries": 567890,
    "block_percentage": 10.0,
    "cache_hit_rate": 46.0,
    "clients_active": 25,
    "uptime_seconds": 86400
}

GET /api/stats/hourly?days=7
Response: [
    {
        "timestamp": "2025-01-15T10:00:00Z",
        "total": 1500,
        "blocked": 200,
        "cached": 700
    },
    ...
]

GET /api/queries?limit=100&offset=0&client=192.168.1.100&status=blocked
Response: {
    "queries": [
        {
            "timestamp": "2025-01-15T10:30:45Z",
            "client_ip": "192.168.1.100",
            "client_name": "John's Laptop",
            "domain": "ads.example.com",
            "query_type": "A",
            "decision": "BLOCK",
            "status": "BLOCKED",
            "cached": false,
            "duration_ms": 2,
            "matched_rule": "Block ads"
        },
        ...
    ],
    "total": 5432,
    "limit": 100,
    "offset": 0
}

GET /api/top-domains?limit=50&period=24h
Response: [
    {
        "domain": "google.com",
        "queries": 1234,
        "percentage": 5.5
    },
    ...
]

GET /api/top-clients?limit=20&period=7d
Response: [
    {
        "client_ip": "192.168.1.100",
        "client_name": "John's Laptop",
        "queries": 5678,
        "blocked": 234
    },
    ...
]

GET /api/top-blocked?limit=50
Response: [
    {
        "domain": "doubleclick.net",
        "blocked_count": 567
    },
    ...
]
```

### Client Management

```
GET /api/clients
Response: [
    {
        "id": "client-uuid-1",
        "ip": "192.168.1.100",
        "mac": "aa:bb:cc:dd:ee:ff",
        "name": "John's Laptop",
        "groups": ["family", "adults"],
        "tags": ["laptop", "trusted"],
        "last_seen": "2025-01-15T10:30:45Z",
        "total_queries": 5678,
        "enabled": true
    },
    ...
]

GET /api/clients/:id
Response: {
    "id": "client-uuid-1",
    "ip": "192.168.1.100",
    "name": "John's Laptop",
    "groups": ["family", "adults"],
    "settings": {
        "rate_limit": {
            "requests_per_second": 100,
            "burst": 200
        },
        "log_queries": true
    },
    "stats": {
        "total_queries": 5678,
        "blocked_queries": 234,
        "last_query_time": "2025-01-15T10:30:45Z",
        "top_domains": [...]
    }
}

POST /api/clients
Request: {
    "name": "New Device",
    "ip": "192.168.1.200",
    "groups": ["family"],
    "settings": {...}
}

PUT /api/clients/:id
Request: {
    "name": "Updated Name",
    "groups": ["family", "kids"],
    ...
}

DELETE /api/clients/:id
Response: 204 No Content
```

### Group Management

```
GET /api/groups
GET /api/groups/:name
POST /api/groups
PUT /api/groups/:name
DELETE /api/groups/:name
```

### Configuration

```
GET /api/config
Response: <full configuration YAML>

PUT /api/config
Request: <updated configuration YAML>
Response: 200 OK or 400 Bad Request with validation errors

POST /api/config/validate
Request: <configuration YAML to validate>
Response: {
    "valid": true|false,
    "errors": ["error message 1", ...]
}
```

### Blocklist Management

```
GET /api/blocklists
Response: [
    {
        "url": "https://...",
        "entries": 123456,
        "last_updated": "2025-01-15T00:00:00Z",
        "next_update": "2025-01-16T00:00:00Z"
    },
    ...
]

POST /api/blocklists/reload
Response: {
    "status": "reloading",
    "job_id": "job-uuid-1"
}

GET /api/blocklists/reload/:job_id
Response: {
    "status": "completed",
    "entries_added": 5678,
    "entries_removed": 234,
    "duration_seconds": 12.5
}

POST /api/blocklists/add
Request: {
    "url": "https://new-blocklist-url"
}

DELETE /api/blocklists
Request: {
    "url": "https://blocklist-to-remove"
}
```

### Local Records Management

```
GET /api/local-records
GET /api/local-records/:id
POST /api/local-records
PUT /api/local-records/:id
DELETE /api/local-records/:id

POST /api/ddns/update
Request: {
    "domain": "dynamic.local",
    "ip": "192.168.1.100",
    "ttl": 300,
    "auth_token": "secret-token"
}
```

### Rate Limiting

```
GET /api/rate-limits/status
Response: {
    "clients": [
        {
            "client_ip": "192.168.1.100",
            "current_rate": 45.2,
            "limit": 50,
            "burst_available": 75,
            "violations_count": 0
        },
        ...
    ]
}

GET /api/rate-limits/violations?hours=24
Response: [
    {
        "timestamp": "2025-01-15T10:15:30Z",
        "client_ip": "192.168.1.150",
        "client_name": "Suspicious Device",
        "requests_attempted": 500,
        "violation_type": "client"
    },
    ...
]

DELETE /api/rate-limits/blocks/:client_ip
Response: 204 No Content (unblock client)
```

### System

```
GET /api/health
Response: {
    "status": "healthy",
    "uptime_seconds": 86400,
    "version": "1.0.0"
}

GET /api/version
Response: {
    "version": "1.0.0",
    "commit": "abc123",
    "build_date": "2025-01-15"
}

POST /api/cache/clear
Response: {
    "entries_cleared": 5432
}

POST /api/reload
Response: {
    "status": "reloaded",
    "message": "Configuration reloaded successfully"
}
```

---

## Implementation Priority

### Phase 1: Foundation (Week 1-2)
1. Client management infrastructure
2. Storage schema and migrations
3. Enhanced configuration loading
4. API framework setup

### Phase 2: Core Features (Week 3-4)
1. Rate limiting implementation
2. Group management
3. Enhanced local records
4. CNAME improvements

### Phase 3: Advanced Features (Week 5-6)
1. Conditional forwarding
2. Policy engine with client/group context
3. Comprehensive API endpoints
4. Query logging and statistics

### Phase 4: Polish (Week 7-8)
1. Web UI for new features
2. Performance optimization
3. Comprehensive testing
4. Documentation

---

## Performance Considerations

### Memory Management
- Use sync.Pool for frequently allocated objects
- Implement LRU cache eviction
- Limit in-memory client/group storage
- Periodic cleanup of stale entries

### Concurrency
- Lock-free reads where possible (sync.Map for hot paths)
- Coarse-grained locking for writes
- Background goroutines for statistics aggregation
- Connection pooling for database

### Optimization Targets
- DNS query latency: < 5ms (cache hit), < 50ms (upstream)
- Memory usage: < 100MB for 10k clients
- Throughput: > 10k queries/second on modern hardware
- Cache hit rate: > 40%

### Monitoring Metrics
- Query rate (queries/second)
- Cache hit rate
- Block rate
- Average response time
- Client count
- Memory usage
- Database write buffer size

---

## Security Considerations

### DNS Security
- DNSSEC validation (future)
- DNS-over-HTTPS (DoH) support (future)
- DNS-over-TLS (DoT) support (future)
- Rate limiting to prevent amplification attacks
- Query validation and sanitization

### API Security
- JWT authentication
- Role-based access control (RBAC)
- API rate limiting
- Input validation
- HTTPS only (optional)

### Configuration Security
- Validate all user inputs
- Prevent code injection in policy rules
- Sanitize blocklist URLs
- Secure credential storage
- Configuration file permissions

### Privacy
- Option to disable query logging
- Query log retention policies
- Anonymization options
- GDPR compliance considerations

---

## Testing Strategy

### Unit Tests
- Each manager component (client, group, rate limiter, etc.)
- Configuration parsing and validation
- Policy engine expression evaluation
- Record matching algorithms

### Integration Tests
- End-to-end DNS query flow
- Database operations
- API endpoints
- Configuration reloading

### Performance Tests
- Benchmark DNS query throughput
- Memory usage profiling
- Cache performance
- Concurrent client stress test

### Test Data
- Mock DNS responses
- Sample blocklists
- Test client/group configurations
- Edge cases and error conditions

---

## Migration Path

### From Current Version
1. Add new configuration fields with defaults
2. Create database schema (new tables)
3. Import existing overrides/blocklists
4. Gradual feature rollout with feature flags
5. Backward compatibility for existing configs

### From Pi-hole
1. Import blocklists
2. Convert whitelist/blacklist
3. Migrate local DNS records
4. Convert group configurations (if using groups)
5. Import client identifiers

### Configuration Versioning
```yaml
config_version: "2.0"
```
- Parser detects version and applies migrations
- Automatic upgrade on save
- Warning if config is too old

---

## Future Enhancements

### Short Term
- IPv6 support improvements
- EDNS Client Subnet (ECS) support
- Response Policy Zones (RPZ)
- Grafana/Prometheus metrics export

### Medium Term
- DNS-over-HTTPS (DoH) server
- DNS-over-TLS (DoT) server
- DNSSEC validation
- Certificate-based client authentication

### Long Term
- Distributed deployment support
- Active Directory integration
- Machine learning for anomaly detection
- Custom plugin system
- Mobile app for management
