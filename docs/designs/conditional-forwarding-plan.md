# Conditional Forwarding Implementation Plan

**Version**: v0.7.0
**Estimated Effort**: 12-18 hours
**Status**: Planning

## Overview

Implement conditional forwarding to route DNS queries to different upstream DNS servers based on:
- Domain patterns (e.g., all `.local` queries)
- Client IP addresses (e.g., VPN clients)
- Query types (e.g., PTR for reverse DNS)
- Time of day / day of week
- Combinations of the above

## Use Cases

### 1. Split-DNS for Local Network
```yaml
# Forward .local domains to router/local DNS
# Everything else to public DNS (1.1.1.1, 8.8.8.8)
conditional_forwarding:
  rules:
    - domains: ["*.local", "*.lan"]
      upstreams: ["10.0.0.1:53"]
```

### 2. Corporate VPN Split-DNS
```yaml
# VPN clients (10.8.0.0/24) get corporate DNS for company domains
# Other clients use public DNS
conditional_forwarding:
  rules:
    - client_cidrs: ["10.8.0.0/24"]
      domains: ["*.company.com", "*.internal"]
      upstreams: ["10.0.0.1:53", "10.0.0.2:53"]
```

### 3. Reverse DNS for Local Networks
```yaml
# PTR queries for local IP ranges go to local DNS
conditional_forwarding:
  rules:
    - domains:
        - "*.10.in-addr.arpa"      # 10.0.0.0/8
        - "*.168.192.in-addr.arpa" # 192.168.0.0/16
      query_types: ["PTR"]
      upstreams: ["10.0.0.1:53"]
```

### 4. Time-Based Routing
```yaml
# During work hours, use corporate DNS filter
# After hours, use default upstream
policy:
  rules:
    - name: "Work hours corporate DNS"
      logic: 'Hour >= 9 && Hour < 17 && DomainEndsWith(Domain, ".com")'
      action: "forward"
      action_data: "10.0.0.1:53"
```

## Dual Implementation Approach

We'll implement BOTH approaches for maximum flexibility:

### Approach 1: Policy Engine FORWARD Action
**When to use**: Complex conditional logic, dynamic rules based on time/context

```yaml
policy:
  enabled: true
  rules:
    - name: "Local domains to local DNS"
      logic: 'DomainEndsWith(Domain, ".local")'
      action: "forward"
      action_data: "10.0.0.1:53"
      enabled: true

    - name: "VPN clients for work domains"
      logic: 'IPInCIDR(ClientIP, "10.8.0.0/24") && DomainEndsWith(Domain, ".corp.com")'
      action: "forward"
      action_data: "10.0.0.1:53,10.0.0.2:53"  # Comma-separated list
      enabled: true
```

**Pros**:
- Leverages existing policy expression language
- Maximum flexibility (can use any helper functions)
- Time-based, client-based, domain-based all in one place
- Users already familiar with policy syntax

**Cons**:
- Slightly more verbose for simple cases
- Expression evaluation overhead (minimal, ~100ns)
- Harder to validate upstream list at config load time

### Approach 2: Declarative Conditional Forwarding
**When to use**: Simple static rules, easier configuration

```yaml
conditional_forwarding:
  enabled: true
  rules:
    - name: "Local network DNS"
      priority: 100              # Higher = evaluated first
      domains:                   # Match any of these patterns
        - "*.local"
        - "*.lan"
        - "homelab.internal"
      client_cidrs:              # Optional: AND with client IP
        - "10.0.0.0/8"
        - "192.168.0.0/16"
      query_types:               # Optional: A, AAAA, PTR, etc.
        - "A"
        - "AAAA"
        - "PTR"
      upstreams:                 # Where to forward
        - "10.0.0.1:53"
        - "10.0.0.2:53"
      failover: true             # Try next upstream on failure
      timeout: 2s                # Per-rule timeout
      enabled: true

    - name: "Reverse DNS"
      priority: 90
      domains:
        - "*.10.in-addr.arpa"
        - "*.168.192.in-addr.arpa"
      upstreams:
        - "10.0.0.1:53"
      enabled: true
```

**Pros**:
- Simpler syntax for common cases
- Easier validation at config load time
- Clear, declarative intent
- Better performance (no expression compilation)
- Priority-based ordering

**Cons**:
- Less flexible (no time-based routing without policy)
- Two configuration sections for forwarding logic

## How They Work Together

**Processing order** (after cache/local_records/policy BLOCK/ALLOW):

```
1. Cache Check
2. Local Records (authoritative answers)
3. Policy Engine Evaluation
   ├─ BLOCK → return NXDOMAIN
   ├─ ALLOW → skip blocklist, continue to step 4
   ├─ REDIRECT → return static IP from action_data
   └─ FORWARD → use upstreams from action_data ← NEW
4. Conditional Forwarding Rules (if no policy FORWARD matched)
   ├─ Evaluate rules by priority
   ├─ First match → use those upstreams
   └─ No match → continue to step 5
5. Blocklist Check
6. Default Upstream Forwarders
```

**Example: Both in use**
```yaml
# Policy for complex logic
policy:
  rules:
    # Dynamic rule: work hours only
    - name: "Work hours DNS filter"
      logic: 'Hour >= 9 && Hour < 17'
      action: "forward"
      action_data: "10.0.0.1:53"
      enabled: true

# Conditional forwarding for static rules
conditional_forwarding:
  rules:
    # Always route .local to router
    - domains: ["*.local"]
      upstreams: ["10.0.0.1:53"]
```

**Conflict resolution**:
- Policy engine FORWARD takes precedence over conditional_forwarding
- If policy FORWARD matches, skip conditional_forwarding rules
- If policy ALLOW matches but no FORWARD, continue to conditional_forwarding

## Design Decisions

### 1. Domain Matching

**Supported patterns**:
- **Exact match**: `nas.local` → matches only `nas.local`
- **Wildcard prefix**: `*.local` → matches `nas.local`, `router.local`, but NOT `local`
- **Wildcard suffix**: `internal.*` → matches `internal.corp`, `internal.net`
- **Regex**: `/^[a-z]+\.local$/` → advanced matching (optional, phase 2)

**Implementation**:
```go
type DomainMatcher struct {
    exact    map[string]struct{}  // Exact matches
    suffixes []string              // Wildcard suffixes (*.local)
    prefixes []string              // Wildcard prefixes (internal.*)
    regexes  []*regexp.Regexp      // Regex patterns (optional)
}

func (dm *DomainMatcher) Matches(domain string) bool {
    // 1. Check exact match (O(1))
    if _, ok := dm.exact[domain]; ok {
        return true
    }

    // 2. Check suffix wildcards (O(n), but n is small)
    for _, suffix := range dm.suffixes {
        if strings.HasSuffix(domain, suffix) {
            return true
        }
    }

    // 3. Check prefix wildcards
    for _, prefix := range dm.prefixes {
        if strings.HasPrefix(domain, prefix) {
            return true
        }
    }

    // 4. Check regex (slow, optional)
    for _, re := range dm.regexes {
        if re.MatchString(domain) {
            return true
        }
    }

    return false
}
```

### 2. Priority System

**Conditional forwarding rules**:
- Higher priority = evaluated first
- Default priority: 50
- Range: 1-100
- Most specific should have higher priority

**Example priority assignment**:
```yaml
conditional_forwarding:
  rules:
    - name: "Specific host"
      priority: 100
      domains: ["nas.local"]  # Most specific
      upstreams: ["10.0.0.100:53"]

    - name: "Local domain wildcard"
      priority: 80
      domains: ["*.local"]    # Less specific
      upstreams: ["10.0.0.1:53"]

    - name: "All domains for VPN"
      priority: 50
      client_cidrs: ["10.8.0.0/24"]  # Least specific
      upstreams: ["10.0.0.1:53"]
```

**Policy engine**:
- Always evaluated before conditional_forwarding
- Rules evaluated in order added
- First match wins

### 3. Caching Strategy

**Options**:

**A. Global cache (current behavior)**:
- Single cache for all upstreams
- Pros: Simpler, better cache hit rate
- Cons: Can't respect different TTLs from different upstreams

**B. Per-upstream cache**:
- Separate cache per upstream or per rule
- Pros: Respects TTLs from each upstream
- Cons: More complex, lower cache hit rate

**Decision**: Start with **global cache**, add per-upstream in v0.8.0 if needed

**Cache key includes**:
```go
type CacheKey struct {
    Domain    string
    QueryType uint16
    // Upstream  string  // Add this for per-upstream cache
}
```

### 4. Failover & Retries

**Per-rule failover**:
```yaml
conditional_forwarding:
  rules:
    - upstreams:
        - "10.0.0.1:53"  # Primary
        - "10.0.0.2:53"  # Fallback
      failover: true     # Try next on failure
      max_retries: 2     # Per upstream
```

**Implementation**:
- Try each upstream in order
- Failover on: timeout, SERVFAIL, connection refused
- Don't failover on: NXDOMAIN (legitimate response)

### 5. Action Data Format for Policy FORWARD

**Options for multiple upstreams**:

**A. Comma-separated string**:
```yaml
action_data: "10.0.0.1:53,10.0.0.2:53"
```
Pros: Simple, consistent with other actions
Cons: No per-upstream config (timeout, priority)

**B. JSON string**:
```yaml
action_data: '{"upstreams": ["10.0.0.1:53", "10.0.0.2:53"], "timeout": "2s"}'
```
Pros: Extensible, structured
Cons: More complex to parse and validate

**Decision**: Start with **comma-separated**, add JSON in future if needed

## Implementation Plan

### Phase 1: Core Infrastructure (4-6 hours)

**1.1 Add FORWARD action to policy engine** (2 hours)
- Add `ActionForward = "FORWARD"` constant
- Parse action_data as comma-separated upstream list
- Add validation at rule compile time

**1.2 Create conditional forwarding config structs** (1 hour)
```go
// pkg/config/conditional_forwarding.go
type ConditionalForwardingConfig struct {
    Enabled bool                      `yaml:"enabled"`
    Rules   []ForwardingRule          `yaml:"rules"`
}

type ForwardingRule struct {
    Name        string        `yaml:"name"`
    Priority    int           `yaml:"priority"`
    Domains     []string      `yaml:"domains"`
    ClientCIDRs []string      `yaml:"client_cidrs"`
    QueryTypes  []string      `yaml:"query_types"`
    Upstreams   []string      `yaml:"upstreams"`
    Failover    bool          `yaml:"failover"`
    Timeout     time.Duration `yaml:"timeout"`
    MaxRetries  int           `yaml:"max_retries"`
    Enabled     bool          `yaml:"enabled"`
}
```

**1.3 Domain matching engine** (2 hours)
- Implement `DomainMatcher` struct
- Support exact, wildcard prefix (`*.local`), wildcard suffix
- Benchmark matching performance

**1.4 Rule evaluation engine** (1 hour)
- Sort rules by priority
- Match domain, client IP, query type
- Return matched upstreams or nil

### Phase 2: DNS Handler Integration (3-4 hours)

**2.1 Modify DNS handler** (2 hours)
- After policy engine, check for FORWARD action
- If FORWARD, parse upstreams and use them instead of default
- After policy (if no FORWARD), evaluate conditional_forwarding rules
- Pass matched upstreams to forwarder

**2.2 Forwarder enhancement** (2 hours)
- Add `ForwardWithUpstreams(ctx, msg, upstreams []string)` method
- Reuse existing retry/failover logic
- Per-query upstream selection

```go
// pkg/forwarder/forwarder.go
func (f *Forwarder) ForwardWithUpstreams(ctx context.Context, r *dns.Msg, upstreams []string) (*dns.Msg, error) {
    // Use provided upstreams instead of f.upstreams
    // Rest of logic identical to Forward()
}
```

### Phase 3: Configuration & Validation (2-3 hours)

**3.1 Config loading** (1 hour)
- Load `ConditionalForwardingConfig` from YAML
- Add to main `Config` struct
- Validate rules at startup

**3.2 Validation** (1 hour)
- Validate upstream addresses (host:port format)
- Validate CIDR notation
- Validate domain patterns (no invalid wildcards)
- Validate query type names
- Check for conflicting rules (warn, don't error)

**3.3 Example configs** (1 hour)
- Update `config.example.yml`
- Add split-DNS example
- Add VPN example
- Add reverse DNS example

### Phase 4: Testing (3-4 hours)

**4.1 Unit tests** (2 hours)
- Test domain matching (exact, wildcard)
- Test rule priority ordering
- Test CIDR matching
- Test FORWARD action parsing

**4.2 Integration tests** (2 hours)
- Test conditional forwarding with mock upstreams
- Test policy FORWARD action
- Test precedence (policy > conditional > default)
- Test failover scenarios
- Test with real local DNS server

### Phase 5: Documentation (1-2 hours)

**5.1 Update docs**
- Add conditional forwarding section to README
- Update DNS processing order diagram
- Add troubleshooting guide

**5.2 Migration guide**
- How to migrate from policy REDIRECT to FORWARD
- How to combine with local_records
- Performance considerations

## Configuration Examples

### Example 1: Home Network Split-DNS
```yaml
# Simple home setup: .local to router, everything else to Cloudflare
upstream_dns_servers:
  - "1.1.1.1:53"
  - "1.0.0.1:53"

conditional_forwarding:
  enabled: true
  rules:
    - name: "Local network DNS"
      domains:
        - "*.local"
        - "*.lan"
      upstreams:
        - "192.168.1.1:53"  # Router
```

### Example 2: Corporate VPN
```yaml
# VPN clients get corporate DNS for company domains
upstream_dns_servers:
  - "1.1.1.1:53"
  - "8.8.8.8:53"

conditional_forwarding:
  enabled: true
  rules:
    - name: "Corporate DNS for VPN clients"
      priority: 100
      client_cidrs:
        - "10.8.0.0/24"  # VPN range
      domains:
        - "*.company.com"
        - "*.internal"
      upstreams:
        - "10.0.0.1:53"
        - "10.0.0.2:53"
      failover: true
```

### Example 3: Reverse DNS
```yaml
conditional_forwarding:
  enabled: true
  rules:
    - name: "Reverse DNS for local networks"
      domains:
        - "*.10.in-addr.arpa"          # 10.0.0.0/8
        - "*.168.192.in-addr.arpa"     # 192.168.0.0/16
        - "*.16.172.in-addr.arpa"      # 172.16.0.0/12
      query_types:
        - "PTR"
      upstreams:
        - "10.0.0.1:53"
```

### Example 4: Combined Policy + Conditional
```yaml
# Use policy for complex time-based logic
policy:
  enabled: true
  rules:
    - name: "Work hours: filter through corporate DNS"
      logic: 'Hour >= 9 && Hour < 17 && !DomainEndsWith(Domain, ".local")'
      action: "forward"
      action_data: "10.0.0.1:53"
      enabled: true

# Use conditional forwarding for simple static rules
conditional_forwarding:
  enabled: true
  rules:
    - name: "Always route .local to router"
      domains: ["*.local"]
      upstreams: ["192.168.1.1:53"]

    - name: "Always route reverse DNS locally"
      domains: ["*.in-addr.arpa"]
      query_types: ["PTR"]
      upstreams: ["192.168.1.1:53"]
```

## Performance Considerations

### Domain Matching Performance

**Expected performance** (based on similar implementations):
- Exact match: O(1) - hash map lookup (~10ns)
- Wildcard match: O(n) - string comparison (~50ns per rule)
- Regex match: O(m) - regex engine (~500ns-1μs)

**Optimization strategies**:
1. Hash map for exact domains
2. Trie data structure for wildcard prefixes (future)
3. Limit regex to power users (or disable by default)

**Benchmark targets**:
- Rule evaluation: < 200ns per query
- Total forwarding overhead: < 1μs
- Memory per rule: < 1KB

### Caching Impact

**Current cache hit rate**: ~60-70% (from load tests)

**With conditional forwarding**:
- Same cache hit rate (global cache)
- Per-upstream cache would reduce hit rate
- Monitor cache effectiveness metrics

## Open Questions

1. **Should we support upstream groups/pools?**
   ```yaml
   upstream_groups:
     corporate:
       - "10.0.0.1:53"
       - "10.0.0.2:53"
     public:
       - "1.1.1.1:53"
       - "8.8.8.8:53"

   conditional_forwarding:
     rules:
       - domains: ["*.local"]
         upstream_group: "corporate"  # Reference group
   ```

2. **Telemetry: What metrics to track?**
   - Per-rule query counts
   - Per-upstream query counts
   - Failover counts
   - Rule matching performance

3. **Should policy FORWARD support failover?**
   Currently: comma-separated list, no failover config
   Future: JSON format with failover settings?

4. **Health checking for conditional upstreams?**
   - Periodic health checks
   - Mark unhealthy upstreams as down
   - Automatic failover

5. **DNS64 support?**
   - Synthesize AAAA from A records
   - Useful for IPv6-only networks

## Success Criteria

- [ ] Policy FORWARD action works with comma-separated upstreams
- [ ] Conditional forwarding rules evaluated by priority
- [ ] Domain patterns (exact, wildcard) work correctly
- [ ] Client CIDR matching works
- [ ] Query type filtering works
- [ ] Failover between upstreams works
- [ ] All tests pass with race detector
- [ ] No performance regression (< 1μs overhead)
- [ ] Documentation complete with examples
- [ ] Example configs for common scenarios

## Future Enhancements (v0.8.0+)

- Regex domain matching
- Per-upstream caching
- Health checks for upstreams
- Upstream groups/pools
- DNS64 support
- Load balancing (round-robin, least-loaded)
- Response manipulation (TTL override, answer filtering)
- Rate limiting per upstream
- Upstream statistics dashboard
