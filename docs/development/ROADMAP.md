# Glory-Hole DNS Server Roadmap

## Current Release: v0.6.1 (Cleanup Release)
**Status**: Released
**Date**: 2025-11-22

## Next Release: v0.7.0 (Conditional Forwarding)
**Status**: ✅ Implementation Complete - Ready for Release
**Target Date**: TBD
**Implementation**: See `v0.7.0-conditional-forwarding-implementation.md`

### Features
- ✅ Conditional DNS forwarding with dual approach (declarative + policy)
- ✅ Domain pattern matching (exact, wildcard, regex)
- ✅ Client IP-based routing (CIDR support)
- ✅ Query type filtering (A, AAAA, PTR, etc.)
- ✅ Priority-based rule evaluation (1-100)
- ✅ Policy engine FORWARD action
- ✅ Sub-200ns performance, zero allocations
- ✅ Comprehensive tests (61 tests, 73%+ coverage)
- ✅ Full documentation

### Release Checklist

#### Pre-Release
- [ ] Update version in `cmd/glory-hole/main.go`
- [ ] Create CHANGELOG.md entry for v0.7.0
- [ ] Update version in documentation
- [ ] Run full test suite on multiple platforms
- [ ] Build release binaries (Linux, macOS, Windows, ARM)
- [ ] Test Docker image build

#### Release
- [ ] Tag release: `git tag v0.7.0`
- [ ] Push tag: `git push origin v0.7.0`
- [ ] Create GitHub release with binaries
- [ ] Update Docker Hub with new image
- [ ] Publish release notes

#### Post-Release
- [ ] Update README badges
- [ ] Announce on project channels
- [ ] Monitor for issues/bugs
- [ ] Start v0.8.0 planning

---

## Future Releases

## v0.8.0 (Management & Observability)
**Status**: Planning
**Target Date**: Q1 2026
**Focus**: Enhanced management, monitoring, and usability

### Priority Features

#### 1. Web UI for Conditional Forwarding Management
**Complexity**: Medium | **Priority**: High

Currently, conditional forwarding rules must be configured via YAML. Add UI management:

- [ ] View all conditional forwarding rules in web UI
- [ ] Create/edit/delete rules via UI forms
- [ ] Enable/disable rules with toggle
- [ ] Test rule matching with query simulator
- [ ] Visual priority ordering (drag-and-drop)
- [ ] Rule statistics (match count, last matched)

**Files to Modify**:
- `pkg/api/handlers.go` - Add rule management endpoints
- `web/templates/` - Add rule management pages
- `pkg/forwarder/evaluator.go` - Add metrics tracking

**API Endpoints**:
```
GET    /api/conditional-forwarding/rules
POST   /api/conditional-forwarding/rules
PUT    /api/conditional-forwarding/rules/:id
DELETE /api/conditional-forwarding/rules/:id
GET    /api/conditional-forwarding/stats
```

#### 2. Conditional Forwarding Metrics & Statistics
**Complexity**: Low | **Priority**: High

Track and expose metrics for conditional forwarding:

- [ ] Rule match counters (per rule)
- [ ] Last matched timestamp
- [ ] Query count per upstream
- [ ] Average response times per rule
- [ ] Failed forwarding attempts
- [ ] Prometheus metrics integration

**Metrics to Add**:
```go
conditionalForwardingRuleMatches    counter  {rule_name}
conditionalForwardingUpstreamQueries counter  {upstream, rule_name}
conditionalForwardingResponseTime   histogram {upstream, rule_name}
conditionalForwardingErrors         counter  {upstream, error_type}
```

**Files to Modify**:
- `pkg/forwarder/evaluator.go` - Add instrumentation
- `pkg/telemetry/metrics.go` - Register new metrics
- `pkg/dns/server.go` - Increment counters

#### 3. Hot Configuration Reload
**Complexity**: High | **Priority**: Medium

Enable config changes without restart:

- [ ] Watch config file for changes
- [ ] Reload conditional forwarding rules dynamically
- [ ] Reload policy engine rules
- [ ] Reload blocklists without service interruption
- [ ] Validate config before applying
- [ ] Rollback on validation errors
- [ ] Signal handlers (SIGHUP to reload)

**Implementation Notes**:
- Use `fsnotify` for file watching (already imported)
- Atomic pointer swaps for lock-free updates
- Graceful rule transition (finish in-flight queries)

**Files to Modify**:
- `cmd/glory-hole/main.go` - Add signal handling
- `pkg/config/watcher.go` - Extend for conditional forwarding
- `pkg/forwarder/evaluator.go` - Add atomic rule updates

#### 4. Enhanced Logging & Debugging
**Complexity**: Low | **Priority**: Medium

Improve observability for conditional forwarding:

- [ ] Log rule matches at DEBUG level
- [ ] Show which rule matched in query logs
- [ ] Trace upstream selection
- [ ] Log performance metrics (evaluation time)
- [ ] Add query flow visualization in UI
- [ ] Export query logs with rule info

**Example Log Output**:
```
DEBUG Conditional forwarding rule matched rule="Local domains"
      domain=nas.local client=192.168.1.50 upstream=192.168.1.1:53
```

#### 5. Rule Validation & Testing Tools
**Complexity**: Medium | **Priority**: Low

Help users test and validate rules:

- [ ] CLI command to test rule matching: `glory-hole test-rule --domain=nas.local --client=192.168.1.1`
- [ ] Dry-run mode to see which rules would match
- [ ] Rule conflict detection (overlapping patterns)
- [ ] Performance analysis (slow rules)
- [ ] Configuration linting
- [ ] Interactive rule builder in UI

### Nice-to-Have Features

#### 6. Advanced Pattern Matching
**Complexity**: Medium | **Priority**: Low

Enhance pattern matching capabilities:

- [ ] Negative patterns: `!ads.*` (exclude ads subdomain)
- [ ] Multiple domain lists: `domains_file: /etc/glory-hole/corporate-domains.txt`
- [ ] Pattern groups: `@corporate = [*.corp, *.internal]`
- [ ] Regex named groups for upstream selection
- [ ] GeoIP-based routing (match by country)

#### 7. Failover & Health Checks
**Complexity**: High | **Priority**: Low

Add upstream health monitoring:

- [ ] Periodic upstream health checks
- [ ] Automatic failover to backup upstreams
- [ ] Mark upstreams as down/degraded
- [ ] Health check metrics
- [ ] Configurable health check intervals
- [ ] DNS-based health checks vs ICMP ping

#### 8. Query Response Time Optimization
**Complexity**: Medium | **Priority**: Low

Optimize conditional forwarding performance:

- [ ] Cache conditional forwarding results separately
- [ ] Upstream connection pooling
- [ ] Parallel upstream queries (race mode)
- [ ] TTL-based upstream selection (prefer faster upstream)
- [ ] Adaptive timeout based on upstream performance

---

## v0.9.0 (Advanced DNS Features)
**Status**: Concept
**Target Date**: Q2 2026
**Focus**: Advanced DNS capabilities

### Potential Features

1. **DNS-over-HTTPS (DoH) Support**
   - Support DoH upstreams (1.1.1.1, 8.8.8.8)
   - HTTP/2 with connection reuse
   - Certificate validation
   - Performance monitoring

2. **DNS-over-TLS (DoT) Support**
   - TLS 1.3 for upstream connections
   - Certificate pinning
   - SNI support

3. **DNSSEC Validation**
   - Validate DNSSEC signatures
   - Chain of trust verification
   - Serve DNSSEC records

4. **Response Policy Zones (RPZ)**
   - Import RPZ feeds
   - Custom RPZ rules
   - RPZ priority configuration

5. **DNS64 Support**
   - Synthesize AAAA from A records
   - Configurable prefix
   - Useful for IPv6-only networks

---

## v1.0.0 (Stability & Performance)
**Status**: Vision
**Target Date**: Q3-Q4 2026
**Focus**: Production hardening

### Goals

1. **Performance Optimization**
   - 1M+ QPS on modern hardware
   - Sub-microsecond cache lookups
   - Zero-copy networking
   - NUMA-aware memory allocation

2. **High Availability**
   - Cluster support (primary/replica)
   - State synchronization
   - Automatic failover
   - Load balancing

3. **Enterprise Features**
   - Multi-tenancy support
   - Rate limiting per client
   - Quota management
   - Advanced RBAC for API

4. **Comprehensive Monitoring**
   - Grafana dashboard templates
   - Alert templates (Alertmanager)
   - SLA monitoring
   - Capacity planning tools

5. **Production Hardening**
   - Formal security audit
   - Fuzzing (DNS protocol)
   - Chaos engineering tests
   - Long-running stability tests (30+ days)

---

## Long-Term Vision (v2.0+)

### Potential Directions

1. **Cloud-Native Features**
   - Kubernetes operator
   - Service mesh integration
   - Multi-cluster DNS
   - Edge deployment support

2. **Machine Learning**
   - Anomaly detection (DDoS, DNS tunneling)
   - Intelligent caching (predict queries)
   - Automatic blocklist curation
   - Pattern recognition for threats

3. **Extended Protocols**
   - DNS over QUIC (DoQ)
   - Oblivious DoH (ODoH)
   - DNSCrypt support

4. **Advanced Security**
   - Real-time threat intelligence feeds
   - Botnet C&C detection
   - Malware domain generation algorithm (DGA) detection
   - Integration with SIEM systems

---

## Community Requests & Feedback

### High-Demand Features
- [ ] Docker Compose examples (in progress)
- [ ] Kubernetes Helm charts (partially complete)
- [ ] ARM64 optimizations
- [ ] Windows service support
- [ ] macOS launchd support

### Feature Requests
Track in: `docs/development/feature-requests.md` (to be created)

### Bug Reports
Track in GitHub Issues

---

## Decision Framework

When prioritizing features, consider:

1. **User Impact**: How many users benefit?
2. **Complexity**: Implementation difficulty (Low/Medium/High)
3. **Performance Impact**: Does it affect query latency?
4. **Maintenance Burden**: Long-term support cost
5. **Compatibility**: Breaking changes?
6. **Security**: Security implications?

---

## Contributing

See `CONTRIBUTING.md` for:
- How to propose features
- Development setup
- Code review process
- Testing requirements

---

## Release Schedule

- **Minor releases** (0.x.0): Every 2-3 months
- **Patch releases** (0.x.y): As needed for bugs
- **Major releases** (x.0.0): Annually

---

## Appendix: Performance Targets

### v0.7.0 Baseline
- Query latency (cache hit): ~100ns
- Query latency (blocklist): ~50ns
- Query latency (forward): ~10ms
- Conditional forwarding: <200ns evaluation
- Throughput: 100K+ QPS (single core)

### v1.0.0 Goals
- Query latency (cache hit): <50ns
- Query latency (blocklist): <20ns
- Query latency (forward): <5ms
- Throughput: 1M+ QPS (multi-core)
- Memory efficiency: <100MB for 10M blocklist entries
