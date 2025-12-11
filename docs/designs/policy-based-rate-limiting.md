# Policy-Based Rate Limiting Design

## Problem Statement

The current rate limiting system (`pkg/ratelimit/manager.go`) is rigid and limited:

**Current Limitations:**
- Only matches by client IP or CIDR range
- Single global rate limit with simple overrides
- Cannot rate limit specific domains, query types, or patterns
- No time-based rate limiting (e.g., stricter at night)
- Cannot combine conditions (e.g., "kids' devices + gaming domains")
- Limited to per-client buckets

**What We Have:**
```yaml
rate_limiting:
  enabled: true
  requests_per_second: 50
  burst: 100
  overrides:
    - name: "kids-devices"
      clients: ["192.168.1.10", "192.168.1.11"]
      requests_per_second: 10
```

**What We Need:**
```yaml
# Use the policy engine's expression language
policies:
  - name: "Rate Limit Gaming Domains"
    logic: 'DomainEndsWith(Domain, ".twitch.tv") || DomainEndsWith(Domain, ".epicgames.com")'
    action: "RATE_LIMIT"
    action_data: "rps=5,burst=10,action=nxdomain,bucket=client"

  - name: "Strict Limits During Sleep Hours"
    logic: 'InTimeRange(Hour, Minute, 23, 0, 6, 0) && IPInCIDR(ClientIP, "192.168.1.0/24")'
    action: "RATE_LIMIT"
    action_data: "rps=2,burst=5,action=drop"

  - name: "Expensive Query Types"
    logic: 'QueryTypeIn(QueryType, "PTR", "ANY", "AXFR")'
    action: "RATE_LIMIT"
    action_data: "rps=1,burst=2,action=drop,bucket=global"
```

## Current Architecture

### Rate Limiting System
**Location:** `pkg/ratelimit/manager.go`

```
┌─────────────────────────────────────┐
│    Rate Limit Manager               │
│  - Token bucket per client IP       │
│  - Global limit + IP/CIDR overrides │
│  - Simple action: drop|nxdomain     │
└─────────────────────────────────────┘
         ↓
    Decision: Allow | Drop
```

### Policy Engine
**Location:** `pkg/policy/engine.go`

```
┌─────────────────────────────────────────┐
│         Policy Engine                   │
│  - Expression-based matching            │
│  - Rich context (domain, IP, time, etc)│
│  - Actions: BLOCK|ALLOW|REDIRECT|etc    │
└─────────────────────────────────────────┘
         ↓
    Rule Match → Action
```

### Current Integration
The policy engine already has a `RATE_LIMIT` action, but it just delegates to the global rate limiter:

```go
// pkg/dns/handler_policy.go:250-273
func (h *Handler) handlePolicyRateLimit(...) {
    if h.RateLimiter == nil {
        return false
    }
    // Just calls the global rate limiter - no customization!
    return h.enforceRateLimit(...)
}
```

## Proposed Architecture

### Design Goals
1. **Extensibility**: Use policy expressions for rate limit matching
2. **Composability**: Multiple rate limit rules with different buckets
3. **Backward Compatibility**: Keep existing config working
4. **Performance**: Minimal overhead, efficient bucket management
5. **Observability**: Clear trace/metrics for policy-based rate limits

### New Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    DNS Query Processing                         │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                      Policy Engine                              │
│  Evaluates rules in order until match                           │
└─────────────────────────────────────────────────────────────────┘
                              ↓
          ┌───────────────────┴───────────────────┐
          ↓                                       ↓
┌──────────────────────────┐       ┌──────────────────────────┐
│  Action: RATE_LIMIT      │       │  Other Actions           │
│  action_data:            │       │  - BLOCK                 │
│    "rps=5,burst=10,..."  │       │  - ALLOW                 │
└──────────────────────────┘       │  - REDIRECT              │
          ↓                         │  - FORWARD               │
┌──────────────────────────────────┐└──────────────────────────┘
│  Policy Rate Limiter Manager     │
│  - Parses action_data            │
│  - Maintains per-rule buckets    │
│  - Bucket strategies:            │
│    * client: per-client-IP       │
│    * rule: per-rule (global)     │
│    * domain: per-domain          │
│    * client+domain: composite    │
└──────────────────────────────────┘
          ↓
    Allow | Deny
```

### Implementation Plan

#### 1. Action Data Format
```
action_data: "rps=<float>,burst=<int>,action=<drop|nxdomain>[,bucket=<strategy>]"

Examples:
- "rps=10,burst=20,action=nxdomain"  // Default bucket=client
- "rps=1,burst=5,action=drop,bucket=rule"  // Global per-rule bucket
- "rps=5,burst=10,action=nxdomain,bucket=domain"  // Per-domain bucket
- "rps=2,burst=5,action=drop,bucket=client+domain"  // Composite bucket
```

#### 2. Bucket Strategies

**client** (default):
- One bucket per client IP
- Same as current behavior
- Use case: General per-client rate limiting

**rule**:
- One bucket per rule (global)
- Shared across all clients
- Use case: "Limit expensive queries globally to 100/sec"

**domain**:
- One bucket per domain
- Use case: "Limit queries to specific domain regardless of client"

**client+domain**:
- One bucket per (client, domain) pair
- Use case: "Each client can query example.com max 5 times/sec"

#### 3. New Components

**`pkg/policy/ratelimit_config.go`**
```go
type RateLimitConfig struct {
    RequestsPerSecond float64
    Burst             int
    Action            config.RateLimitAction  // drop | nxdomain
    BucketStrategy    BucketStrategy
}

type BucketStrategy string
const (
    BucketClient       BucketStrategy = "client"
    BucketRule         BucketStrategy = "rule"
    BucketDomain       BucketStrategy = "domain"
    BucketClientDomain BucketStrategy = "client+domain"
)

// ParseRateLimitConfig parses action_data string
func ParseRateLimitConfig(actionData string) (*RateLimitConfig, error)
```

**`pkg/policy/ratelimit_manager.go`**
```go
type PolicyRateLimiter struct {
    mu      sync.RWMutex
    buckets map[string]*rate.Limiter
    configs map[string]*RateLimitConfig  // rule name -> config

    cleanupInterval time.Duration
    maxBuckets      int
    now             func() time.Time
}

func (p *PolicyRateLimiter) Allow(
    rule *Rule,
    clientIP string,
    domain string,
) (allowed bool, limited bool, action config.RateLimitAction)

// Returns bucket key based on strategy
func (p *PolicyRateLimiter) bucketKey(
    strategy BucketStrategy,
    ruleName string,
    clientIP string,
    domain string,
) string
```

#### 4. Integration Points

**`pkg/policy/engine.go`** - Add validation:
```go
case ActionRateLimit:
    if rule.ActionData == "" {
        return fmt.Errorf("RATE_LIMIT requires action_data")
    }
    _, err := ParseRateLimitConfig(rule.ActionData)
    if err != nil {
        return fmt.Errorf("invalid rate limit config: %w", err)
    }
    return nil
```

**`pkg/dns/handler_policy.go`** - Update handler:
```go
func (h *Handler) handlePolicyRateLimit(..., rule *policy.Rule, ...) bool {
    // Parse rate limit config from rule.ActionData
    config, err := policy.ParseRateLimitConfig(rule.ActionData)
    if err != nil {
        h.Logger.Error("Invalid rate limit config", "rule", rule.Name, "error", err)
        return false
    }

    // Check policy-based rate limiter
    allowed, limited, action := h.PolicyRateLimiter.Allow(rule, clientIP, domain)

    if limited {
        // Handle rate limit exceeded
        trace.Record(traceStageRateLimit, string(action), func(entry *storage.BlockTraceEntry) {
            entry.Rule = rule.Name
            entry.Source = "policy_rate_limiter"
            entry.Metadata = map[string]string{
                "bucket_strategy": string(config.BucketStrategy),
            }
        })

        // Apply action (drop or nxdomain)
        // ...
        return true
    }

    return false
}
```

## Bucket Key Strategies

```go
func bucketKey(strategy BucketStrategy, ruleName, clientIP, domain string) string {
    switch strategy {
    case BucketClient:
        return fmt.Sprintf("rl:c:%s", clientIP)
    case BucketRule:
        return fmt.Sprintf("rl:r:%s", ruleName)
    case BucketDomain:
        return fmt.Sprintf("rl:d:%s", domain)
    case BucketClientDomain:
        return fmt.Sprintf("rl:cd:%s:%s", clientIP, domain)
    default:
        return fmt.Sprintf("rl:c:%s", clientIP)  // fallback
    }
}
```

## Usage Examples

### Example 1: Per-Domain Rate Limiting
```yaml
policies:
  - name: "Limit Popular Domains"
    logic: 'DomainEndsWith(Domain, ".youtube.com") || DomainEndsWith(Domain, ".netflix.com")'
    action: "RATE_LIMIT"
    action_data: "rps=100,burst=200,action=nxdomain,bucket=domain"
    enabled: true
```
**Result:** All clients combined can query YouTube/Netflix max 100 req/sec

### Example 2: Time-Based Rate Limiting
```yaml
policies:
  - name: "Night Time Restrictions"
    logic: 'InTimeRange(Hour, Minute, 0, 0, 6, 0)'
    action: "RATE_LIMIT"
    action_data: "rps=5,burst=10,action=drop,bucket=client"
    enabled: true
```
**Result:** Between midnight-6am, each client limited to 5 req/sec

### Example 3: Device Group Rate Limiting
```yaml
policies:
  - name: "Kids Devices Gaming Domains"
    logic: 'IPInCIDR(ClientIP, "192.168.1.0/28") && (DomainEndsWith(Domain, ".epicgames.com") || DomainEndsWith(Domain, ".roblox.com"))'
    action: "RATE_LIMIT"
    action_data: "rps=2,burst=5,action=nxdomain,bucket=client+domain"
    enabled: true
```
**Result:** Kids' devices limited to 2 req/sec per gaming domain

### Example 4: Expensive Query Types
```yaml
policies:
  - name: "Limit Expensive Queries"
    logic: 'QueryTypeIn(QueryType, "ANY", "AXFR", "PTR")'
    action: "RATE_LIMIT"
    action_data: "rps=10,burst=20,action=drop,bucket=rule"
    enabled: true
```
**Result:** Max 10 expensive queries/sec globally across all clients

## Performance Considerations

### Memory Management
- **Bucket Cleanup**: Periodic cleanup of idle buckets (similar to current rate limiter)
- **Max Buckets**: Configurable limit to prevent memory exhaustion
- **Bucket Eviction**: LRU eviction when max reached

### Computational Overhead
- **Policy Evaluation**: Already happens, no additional cost
- **Bucket Lookup**: O(1) map lookup per matched rule
- **Token Bucket**: Same golang.org/x/time/rate, very efficient

### Optimization Strategies
1. **Bucket Pooling**: Reuse limiter objects
2. **Index by Action**: Skip non-RATE_LIMIT rules early
3. **Lazy Initialization**: Create buckets on first use
4. **Metrics Caching**: Avoid repeated allocations

## Backward Compatibility

### Keep Existing Rate Limiter
```yaml
rate_limiting:
  enabled: true
  requests_per_second: 50
  burst: 100
```
**Execution Order:**
1. Global rate limiter (if enabled)
2. Policy engine (including RATE_LIMIT actions)
3. Blocklists
4. Forwarding

### Migration Path
1. **Phase 1**: Implement policy-based rate limiting (this design)
2. **Phase 2**: Migrate existing overrides to policies
3. **Phase 3**: Deprecate (but keep) old rate limiter
4. **Phase 4**: Remove old system in v2.0

## Metrics and Observability

### New Metrics
```
dns_policy_rate_limit_exceeded_total{rule="<name>",bucket_strategy="<strategy>",action="<drop|nxdomain>"}
dns_policy_rate_limit_buckets_active{rule="<name>"}
dns_policy_rate_limit_bucket_operations_total{operation="create|evict"}
```

### Trace Information
```json
{
  "stage": "rate_limit",
  "action": "nxdomain",
  "rule": "Night Time Restrictions",
  "source": "policy_rate_limiter",
  "metadata": {
    "bucket_strategy": "client",
    "bucket_key": "rl:c:192.168.1.50"
  }
}
```

### Debug Logging
```
[INFO] Policy rate limit matched rule="Kids Devices Gaming Domains" client=192.168.1.10 domain=roblox.com
[WARN] Rate limit exceeded rule="Kids Devices Gaming Domains" bucket=rl:cd:192.168.1.10:roblox.com action=nxdomain
```

## Trade-offs

### Advantages ✅
- **Maximum Flexibility**: Full expression language power
- **Composability**: Multiple independent rate limit rules
- **Consistency**: Uses same engine as other policies
- **Powerful Patterns**: Combine time, domain, IP, query type
- **Per-Rule Configuration**: Each rule has its own limits
- **Bucket Strategies**: Different sharing semantics

### Disadvantages ❌
- **Complexity**: More moving parts vs simple IP-based limiter
- **Memory**: More buckets = more memory (mitigated by cleanup)
- **Configuration**: More complex YAML (power users only)
- **Learning Curve**: Need to understand bucket strategies

### When to Use Each Approach

**Use Global Rate Limiter:**
- Simple per-client limits across all queries
- Easy to configure
- Minimal memory footprint

**Use Policy-Based Rate Limiter:**
- Need domain/query-type specific limits
- Time-based rate limiting
- Complex conditions (device groups + domain patterns)
- Per-domain or global bucket strategies

## Testing Strategy

### Unit Tests
- `ParseRateLimitConfig()`: Parse all action_data formats
- `bucketKey()`: Generate correct keys for all strategies
- `PolicyRateLimiter.Allow()`: Token bucket enforcement
- Bucket cleanup and eviction logic

### Integration Tests
- End-to-end with real DNS queries
- Multiple rules with different bucket strategies
- Verify metrics and traces
- Concurrent access from multiple clients

### Performance Tests
- Benchmark bucket lookup overhead
- Memory usage with 10k/100k/1M buckets
- Throughput impact vs baseline

## Implementation Checklist

- [ ] Create `pkg/policy/ratelimit_config.go`
  - [ ] `RateLimitConfig` struct
  - [ ] `ParseRateLimitConfig()` with validation
  - [ ] Unit tests for parsing

- [ ] Create `pkg/policy/ratelimit_manager.go`
  - [ ] `PolicyRateLimiter` struct
  - [ ] `Allow()` method with bucket strategies
  - [ ] `bucketKey()` generation
  - [ ] Bucket cleanup goroutine
  - [ ] Unit tests for all bucket strategies

- [ ] Update `pkg/policy/engine.go`
  - [ ] Validate RATE_LIMIT action_data in `validateAction()`
  - [ ] Add tests

- [ ] Update `pkg/dns/handler.go`
  - [ ] Add `PolicyRateLimiter *policy.PolicyRateLimiter` field
  - [ ] Initialize in constructor

- [ ] Update `pkg/dns/handler_policy.go`
  - [ ] Rewrite `handlePolicyRateLimit()` to use policy limiter
  - [ ] Add proper tracing
  - [ ] Add metrics

- [ ] Add metrics in `pkg/dns/metrics.go`
  - [ ] `dns_policy_rate_limit_exceeded_total`
  - [ ] `dns_policy_rate_limit_buckets_active`

- [ ] Documentation
  - [ ] Update policy engine docs
  - [ ] Add rate limiting cookbook
  - [ ] Migration guide from old rate limiter

- [ ] Testing
  - [ ] Unit tests (95%+ coverage)
  - [ ] Integration tests
  - [ ] Performance benchmarks

## Future Enhancements

### Dynamic Rate Limits
```yaml
policies:
  - name: "Adaptive Rate Limiting"
    logic: 'DomainEndsWith(Domain, ".suspicious.com")'
    action: "RATE_LIMIT"
    action_data: "rps=expr:10/(1+ClientFailureRate),burst=5"
```

### Rate Limit Based on Response
```yaml
policies:
  - name: "Limit Based on Upstream Errors"
    logic: 'UpstreamErrorRate > 0.1'
    action: "RATE_LIMIT"
    action_data: "rps=1,burst=2"
```

### Distributed Rate Limiting
- Share rate limit state across multiple Glory-Hole instances
- Redis/etcd backend for rate limit buckets
- Enables true multi-node rate limiting

### Token Bucket Variants
- Leaky bucket algorithm
- Sliding window counters
- Fixed window counters

## Conclusion

This design makes rate limiting as powerful and extensible as the policy engine by:
1. Leveraging the existing expression language
2. Supporting multiple bucket strategies (client, rule, domain, composite)
3. Maintaining backward compatibility with simple rate limiter
4. Providing clear migration path

The implementation is straightforward, performant, and follows existing patterns in the codebase.

**Recommendation: Proceed with implementation** ✅
