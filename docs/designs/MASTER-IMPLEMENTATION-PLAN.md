# Glory-Hole Master Implementation Plan
**Date:** 2025-01-15
**Status:** Planning Phase
**Goal:** Complete extensibility and UI feature parity

## Executive Summary

This master plan combines three major initiatives to make Glory-Hole's DNS filtering system fully extensible and user-friendly:

1. **Policy-Based Rate Limiting** - Make rate limiting as flexible as policies
2. **Missing UI Features** - Add Whitelist, Local Records, and Conditional Forwarding to Web UI
3. **Policy Engine Extensions** - Framework for adding future policy actions

## Core Philosophy

> **"The policy engine should be the universal extensibility point"**

All DNS decision-making should flow through the policy engine, which provides:
- Expression-based matching (domain, IP, time, query type, etc.)
- Pluggable actions (BLOCK, ALLOW, REDIRECT, FORWARD, RATE_LIMIT, and future actions)
- Runtime configuration via UI
- Clear traceability and metrics

## Initiative 1: Policy-Based Rate Limiting

### Current Problem
Rate limiting is rigid - only per-client IP with simple overrides. Cannot rate limit:
- Specific domains (e.g., "limit gaming sites to 5 req/sec")
- Query types (e.g., "limit expensive PTR queries")
- Time-based (e.g., "stricter limits at night")
- Combinations (e.g., "kids' devices on gaming domains")

### Solution
Leverage policy engine for rate limiting with multiple bucket strategies.

**Design Document:** `docs/designs/policy-based-rate-limiting.md`

### Example
```yaml
policies:
  - name: "Rate Limit Gaming Domains"
    logic: 'DomainEndsWith(Domain, ".twitch.tv") || DomainEndsWith(Domain, ".epicgames.com")'
    action: "RATE_LIMIT"
    action_data: "rps=5,burst=10,action=nxdomain,bucket=client+domain"
    enabled: true
```

### Implementation Tasks
- [ ] Create `pkg/policy/ratelimit_config.go` - Config parser
- [ ] Create `pkg/policy/ratelimit_manager.go` - Policy rate limiter
- [ ] Update `pkg/policy/engine.go` - Validate RATE_LIMIT action
- [ ] Update `pkg/dns/handler_policy.go` - Use policy rate limiter
- [ ] Add metrics for policy-based rate limiting
- [ ] Unit tests (95%+ coverage)
- [ ] Integration tests
- [ ] Documentation

**Effort:** 5-7 days
**Priority:** High
**Complexity:** Medium

## Initiative 2: Missing UI Features ✅ COMPLETED

### Current Problem (RESOLVED)
Three major features were only configurable via YAML:
1. **Whitelist** - Allow domains to bypass blocking ✅
2. **Local Records** - Custom DNS records (A, AAAA, CNAME, MX, etc.) ✅
3. **Conditional Forwarding** - Route domains to specific upstreams ✅

All features are now fully available in the Web UI.

### Solution (IMPLEMENTED)
Built full-featured UI pages with CRUD operations, validation, and hot reload.

**Design Document:** `docs/archive/plans/ui-missing-features-plan.md` (archived - completed November 2024)
**User Documentation:** `docs/api/web-ui.md` (sections: Whitelist, Local Records, Conditional Forwarding)

### Feature 2.1: Whitelist Management ✅
**Effort:** 2 days | **Priority:** High | **Complexity:** Low | **Status:** COMPLETED

```
UI: /whitelist
- Add/remove domains
- Search and filter
- Bulk import/export
- Show match statistics
```

**Tasks:**
- [x] `pkg/api/handlers_whitelist.go` - API handlers
- [x] `pkg/api/ui/templates/whitelist.html` - UI page
- [x] Config hot reload for whitelist
- [x] Metrics integration
- [x] Tests

### Feature 2.2: Local Records Management ✅
**Effort:** 4 days | **Priority:** Medium | **Complexity:** Medium | **Status:** COMPLETED

```
UI: /local-records
- CRUD for DNS records
- Dynamic forms per record type
- Validation (IP, domain formats)
- Import from zone files
- Test query capability
```

**Tasks:**
- [x] `pkg/api/handlers_local_records.go` - API handlers
- [x] `pkg/api/ui/templates/local_records.html` - UI page
- [x] `pkg/api/ui/static/js/local-records-init.js` - Dynamic forms
- [x] Type-specific validation
- [x] Import/export functionality
- [x] Tests

### Feature 2.3: Conditional Forwarding Management ✅
**Effort:** 5 days | **Priority:** Medium | **Complexity:** High | **Status:** COMPLETED

```
UI: /forwarding
- CRUD for forwarding rules
- Multiple matchers (domains, IPs, query types)
- Priority-based ordering
- Drag-and-drop reordering
- Test rule matching
```

**Tasks:**
- [x] `pkg/api/handlers_conditional_forwarding.go` - API handlers
- [x] `pkg/api/ui/templates/conditional_forwarding.html` - UI page
- [x] `pkg/api/ui/static/js/forwarding-init.js` - Reordering, dynamic forms
- [x] Priority management
- [x] Rule testing endpoint
- [x] Tests

### Common Infrastructure ✅
**Effort:** 2 days | **Status:** COMPLETED

- [x] `pkg/config/writer.go` - Safe config file updates
- [x] Hot reload mechanisms for each feature
- [x] Backup/restore functionality
- [x] Audit logging for config changes

## Initiative 3: Policy Engine Extension Framework

### Current State
Policy engine has fixed actions:
- BLOCK
- ALLOW
- REDIRECT
- FORWARD
- RATE_LIMIT

### Future Extensibility
Create a framework for adding new actions without core changes.

### Proposed Architecture
```go
// pkg/policy/action_registry.go
type ActionHandler interface {
    Name() string
    Validate(actionData string) error
    Execute(ctx Context, actionData string) (bool, error)
}

type ActionRegistry struct {
    handlers map[string]ActionHandler
}

func (r *ActionRegistry) Register(handler ActionHandler)
func (r *ActionRegistry) Execute(action, actionData string, ctx Context) (bool, error)
```

### Future Actions (Examples)
```yaml
# Log specific queries without blocking
- name: "Log Suspicious Domains"
  logic: 'DomainRegex(Domain, ".*\\.xyz$")'
  action: "LOG"
  action_data: "level=warn,tag=suspicious"

# Respond with custom DNS record
- name: "Custom Response"
  logic: 'Domain == "test.local"'
  action: "CUSTOM_RESPONSE"
  action_data: "type=TXT,value=v=spf1 include:_spf.example.com ~all"

# Call webhook on match
- name: "Alert on Corporate Domains"
  logic: 'DomainEndsWith(Domain, ".corp.example.com")'
  action: "WEBHOOK"
  action_data: "url=https://alerts.example.com/dns,method=POST"

# Dynamic TTL based on conditions
- name: "Short TTL for Dev Domains"
  logic: 'DomainEndsWith(Domain, ".dev.local")'
  action: "MODIFY_TTL"
  action_data: "ttl=60"

# Chain multiple actions
- name: "Log and Block"
  logic: 'DomainMatches(Domain, ".malware.com")'
  action: "CHAIN"
  action_data: "actions=LOG,BLOCK"
```

### Implementation Tasks
**Effort:** 3-4 days | **Priority:** Low (Future Enhancement)

- [ ] Create `pkg/policy/action_registry.go`
- [ ] Refactor existing actions to use registry
- [ ] Plugin API for external actions
- [ ] Documentation for custom actions
- [ ] Examples

## Implementation Timeline

### Phase 1: Core Extensibility (Weeks 1-2)
**Focus:** Make rate limiting extensible via policies

```
Week 1:
  Day 1-2:  Design review and finalization
  Day 3-5:  Implement policy-based rate limiting
  Day 6-7:  Testing and documentation

Week 2:
  Day 8-9:  Bug fixes and refinement
  Day 10:   Integration testing
```

**Deliverables:**
- ✅ Policy-based rate limiting fully functional
- ✅ Multiple bucket strategies working
- ✅ Backward compatible with old rate limiter
- ✅ Documentation and examples

### Phase 2: UI Feature Parity (Weeks 3-4)
**Focus:** Add missing UI features

```
Week 3:
  Day 11-12: Whitelist management UI
  Day 13-16: Local records management UI
  Day 17:    Buffer/testing

Week 4:
  Day 18-22: Conditional forwarding UI
  Day 23-24: Config writer + hot reload
```

**Deliverables:**
- ✅ Whitelist management page
- ✅ Local records management page
- ✅ Conditional forwarding management page
- ✅ Config persistence and hot reload
- ✅ Navigation updated
- ✅ All features tested

### Phase 3: Polish & Documentation (Week 5)
**Focus:** Integration, testing, documentation

```
Week 5:
  Day 25-26: End-to-end integration testing
  Day 27-28: User documentation
  Day 29:    Performance testing
  Day 30:    Final review and release prep
```

**Deliverables:**
- ✅ Comprehensive test suite
- ✅ User guides for all features
- ✅ API documentation
- ✅ Migration guides
- ✅ Performance benchmarks

### Phase 4: Future Enhancements (Future)
**Focus:** Extensibility framework

- [ ] Action registry implementation
- [ ] Plugin API for custom actions
- [ ] Example plugins
- [ ] Developer documentation

## Technical Architecture

### System Diagram

```
┌──────────────────────────────────────────────────────────────┐
│                        DNS Query                             │
└──────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────┐
│                  DNS Handler (ServeDNS)                      │
│  - Extract domain, client IP, query type                    │
└──────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────┐
│                   Policy Engine                              │
│  - Evaluate rules in order                                   │
│  - Match using expression language                           │
│  - Execute action if matched                                 │
└──────────────────────────────────────────────────────────────┘
                            ↓
         ┌──────────────────┴──────────────────┐
         ↓                                      ↓
┌──────────────────────┐          ┌──────────────────────────┐
│  RATE_LIMIT Action   │          │  Other Actions           │
│  ↓                   │          │  - BLOCK                 │
│  Policy Rate Limiter │          │  - ALLOW                 │
│  - Parse action_data │          │  - REDIRECT              │
│  - Get/create bucket │          │  - FORWARD               │
│  - Check limit       │          └──────────────────────────┘
│  - Allow or deny     │
└──────────────────────┘
         ↓
    Continue processing
         ↓
┌──────────────────────────────────────────────────────────────┐
│               Whitelist Check (if not matched)               │
└──────────────────────────────────────────────────────────────┘
         ↓
┌──────────────────────────────────────────────────────────────┐
│               Local Records (if matched)                     │
└──────────────────────────────────────────────────────────────┘
         ↓
┌──────────────────────────────────────────────────────────────┐
│               Blocklist Check                                │
└──────────────────────────────────────────────────────────────┘
         ↓
┌──────────────────────────────────────────────────────────────┐
│          Conditional Forwarding / Default Upstream           │
└──────────────────────────────────────────────────────────────┘
```

### Data Flow for Rate Limiting

```
Policy Rule:
  logic: 'DomainEndsWith(Domain, ".gaming.com")'
  action: RATE_LIMIT
  action_data: "rps=5,burst=10,bucket=client+domain"

          ↓

Policy Engine matches rule
          ↓

PolicyRateLimiter.Allow(rule, clientIP, domain)
          ↓

bucketKey = "rl:cd:192.168.1.50:epicgames.com"
          ↓

Get or create rate.Limiter for bucket
          ↓

limiter.Allow() → true/false
          ↓

If false: Execute action (drop/nxdomain)
If true: Continue processing
```

### Config Management Flow

```
User edits config via UI
         ↓
API Handler validates input
         ↓
ConfigWriter.Update*()
         ↓
Atomic write to config.yml.tmp
         ↓
Rename to config.yml (atomic)
         ↓
Backup old config
         ↓
Hot reload subsystem
         ↓
Validation pass
         ↓
Apply changes or rollback
         ↓
Return success/error to UI
```

## Testing Strategy

### Unit Tests
**Coverage Target: 90%+**

- [ ] Policy rate limiter logic
- [ ] Bucket key generation
- [ ] Config parsing and validation
- [ ] API request validation
- [ ] Action data parsing

### Integration Tests
**Coverage: Critical paths**

- [ ] End-to-end DNS query with policy rate limiting
- [ ] Config updates via API
- [ ] Hot reload mechanisms
- [ ] Whitelist + blocklist interaction
- [ ] Local records + forwarding interaction

### Performance Tests
**Benchmarks:**

- [ ] Policy evaluation overhead
- [ ] Rate limiter bucket lookup time
- [ ] Config reload time
- [ ] Memory usage with 10k/100k/1M buckets
- [ ] Query throughput with all features enabled

### UI Tests
**Manual + Automated:**

- [ ] Form validation
- [ ] Modal workflows
- [ ] HTMX partial updates
- [ ] Error handling
- [ ] Responsive design

## Metrics & Observability

### New Metrics

```prometheus
# Policy-based rate limiting
dns_policy_rate_limit_exceeded_total{rule="<name>",bucket_strategy="<strategy>",action="<drop|nxdomain>"}
dns_policy_rate_limit_buckets_active{rule="<name>"}
dns_policy_rate_limit_bucket_operations_total{operation="create|evict"}

# Whitelist
dns_whitelist_matches_total{domain="<domain>"}
dns_whitelist_entries_total

# Local records
dns_local_record_queries_total{domain="<domain>",type="<type>"}
dns_local_records_count{type="<type>"}

# Conditional forwarding
dns_conditional_forwarding_matches_total{rule="<name>"}
dns_conditional_forwarding_forwards_total{rule="<name>",upstream="<upstream>"}
```

### Tracing

All actions should create trace entries:

```json
{
  "stage": "rate_limit",
  "action": "nxdomain",
  "rule": "Gaming Domain Rate Limit",
  "source": "policy_rate_limiter",
  "metadata": {
    "bucket_strategy": "client+domain",
    "bucket_key": "rl:cd:192.168.1.50:epicgames.com",
    "limit": "5 req/s",
    "burst": "10"
  }
}
```

## Documentation Requirements

### User Documentation
- [ ] `/docs/guide/policy-based-rate-limiting.md`
- [ ] `/docs/guide/whitelist-management.md`
- [ ] `/docs/guide/local-records.md`
- [ ] `/docs/guide/conditional-forwarding.md`
- [ ] `/docs/guide/advanced-policies.md`

### API Documentation
- [ ] Update `/docs/api/rest-api.md`
- [ ] OpenAPI spec updates
- [ ] Example requests/responses

### Developer Documentation
- [ ] Policy engine architecture
- [ ] Adding custom policy actions
- [ ] Testing guidelines

## Risk Management

### Risk Matrix

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Config file corruption | High | Low | Atomic writes, backups, validation |
| Performance regression | High | Medium | Benchmarks, profiling, optimization |
| Breaking config changes | High | Low | Backward compatibility, migration scripts |
| Complex UI confusion | Medium | Medium | Progressive disclosure, help text, examples |
| Memory leaks (buckets) | Medium | Medium | Cleanup goroutines, max limits, monitoring |
| Race conditions | Medium | Low | Proper locking, race detector in tests |
| Security vulnerabilities | High | Low | Input validation, auth checks, audit logs |

### Rollback Strategy

If issues arise:
1. **Config rollback**: Restore from automatic backup
2. **Feature toggle**: Disable new features via kill switches
3. **Gradual rollout**: Deploy to staging first, canary rollout
4. **Monitoring**: Alert on errors, high latency, memory usage

## Success Criteria

### Functional Requirements
✅ **Policy-based rate limiting:**
- Multiple bucket strategies work correctly
- Backward compatible with old rate limiter
- Properly traced and metered

✅ **UI feature parity:**
- All three features have full CRUD UIs
- Config persists and hot reloads
- No manual YAML editing required

✅ **Quality:**
- 90%+ test coverage
- No performance regression
- Zero config corruption in testing

### Non-Functional Requirements
✅ **Performance:**
- <1ms overhead per DNS query
- <100ms config reload time
- Memory usage within acceptable bounds

✅ **Usability:**
- Intuitive UI workflows
- Clear error messages
- Comprehensive documentation

✅ **Maintainability:**
- Clean architecture
- Well-tested code
- Clear extension points

## Post-Launch

### Monitoring
- Dashboard for new metrics
- Alerts for anomalies
- Performance tracking

### Feedback Collection
- GitHub issues for bugs
- Feature requests
- User surveys

### Future Enhancements
- Plugin system for custom actions
- GraphQL API
- Advanced analytics dashboard
- Multi-node coordination for rate limiting

## Conclusion

This master plan transforms Glory-Hole from a configurable DNS server into a fully extensible, policy-driven DNS filtering platform with complete UI feature parity.

**Key Benefits:**
1. **Extensibility**: Policy engine becomes universal decision point
2. **Flexibility**: Rate limiting as powerful as blocking rules
3. **Usability**: Everything manageable from Web UI
4. **Maintainability**: Clean architecture for future extensions

**Total Effort:** 30 days (6 weeks)

**Phases:**
- Phase 1 (2 weeks): Policy-based rate limiting
- Phase 2 (2 weeks): UI features (whitelist, local records, forwarding)
- Phase 3 (1 week): Polish and documentation
- Phase 4 (Future): Extension framework

**Recommendation:** Proceed with implementation ✅
