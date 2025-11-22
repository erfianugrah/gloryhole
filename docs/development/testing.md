# Glory-Hole Testing Documentation

**Last Updated**: 2025-11-21  
**Version**: 0.5.0-dev  
**Test Coverage**: 82.5% average across all packages  
**Test Lines**: 9,209 lines of test code

This document describes the testing strategy, test coverage, and how to run tests for Glory-Hole DNS server.

---

## Table of Contents

1. [Overview](#overview)
2. [Test Coverage](#test-coverage)
3. [Running Tests](#running-tests)
4. [Test Categories](#test-categories)
5. [Performance Testing](#performance-testing)
6. [Integration Testing](#integration-testing)
7. [CI/CD Testing](#cicd-testing)
8. [Writing New Tests](#writing-new-tests)

---

## Overview

Glory-Hole uses a comprehensive testing strategy:

- **Unit Tests**: Test individual components in isolation
- **Integration Tests**: Test interactions between components
- **E2E Tests**: Test full DNS server functionality
- **Benchmark Tests**: Measure performance characteristics
- **Race Detection**: Catch concurrency bugs

**Testing Philosophy:**
-  High coverage (>80%) for business logic
-  Race detector enabled in CI
-  Fast tests (<10s total run time)
-  Reliable tests (no flaky tests)
-  Clear test names describing what's being tested

---

## Test Coverage

### Package Coverage Summary

| Package | Coverage | Tests | Test Lines |
|---------|----------|-------|------------|
| `pkg/policy` | 97.0% | 50 | 1,800+ |
| `pkg/localrecords` | 89.9% | 18 | 518 |
| `pkg/blocklist` | 89.8% | 9 | 478 |
| `pkg/config` | 88.5% | 10 | 504 |
| `pkg/cache` | 85.2% | 14 | 605 |
| `pkg/storage` | 77.4% | 13 | 689 |
| `pkg/api` | 75.9% | 42 | 2,073 |
| `pkg/logging` | 72.7% | 9 | 316 |
| `pkg/forwarder` | 72.9% | 5 | 202 |
| `pkg/telemetry` | 70.8% | 7 | 318 |
| `pkg/dns` | 69.8% | 24 | 2,062 |
| `test` (integration) | N/A | 7 | 644 |
| **Total** | **82.5%** | **208** | **9,209** |

### Coverage Goals

- **Critical paths**: >90% coverage (policy, blocklist, local records)
- **Business logic**: >80% coverage (cache, config, storage)
- **Integration points**: >70% coverage (DNS, API, forwarding)
- **Infrastructure**: >60% coverage (logging, telemetry)

---

## Running Tests

### Quick Test

Run all tests:
```bash
go test ./...
```

### With Coverage

Generate coverage report:
```bash
go test -cover ./...
```

### With Race Detector

Detect race conditions:
```bash
go test -race ./...
```

### Verbose Output

See detailed test output:
```bash
go test -v ./...
```

### Specific Package

Test single package:
```bash
go test -v ./pkg/policy
```

### Coverage HTML Report

Generate visual coverage report:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Performance Benchmarks

Run performance benchmarks:
```bash
# All benchmarks
go test -bench=. ./...

# Specific package
go test -bench=. ./pkg/blocklist

# With memory allocations
go test -bench=. -benchmem ./pkg/cache
```

---

## Test Categories

### Unit Tests

Test individual functions/methods in isolation.

**Example**: `pkg/policy/engine_test.go`
```go
func TestEngine_Evaluate_Simple(t *testing.T) {
    engine := NewEngine()
    rule := &Rule{
        Name:    "block-ads",
        Logic:   `domain.endsWith(".ads.com")`,
        Action:  ActionBlock,
        Enabled: true,
    }
    _ = engine.AddRule(rule)
    
    result := engine.Evaluate("tracker.ads.com", "192.168.1.1")
    if result.Action != ActionBlock {
        t.Error("Expected block action")
    }
}
```

**Characteristics:**
- Fast (<1ms per test)
- No external dependencies
- Isolated component testing
- High coverage per test

### Integration Tests

Test interactions between multiple components.

**Location**: `test/integration_test.go`

**Examples:**
- `TestIntegration_DNSWithCache`: DNS + Cache interaction
- `TestIntegration_DNSWithStorage`: DNS + Storage logging
- `TestIntegration_APIWithDNS`: API + DNS management
- `TestIntegration_PolicyEngineRedirect`: Policy + DNS

**Characteristics:**
- Slower (100ms-2s per test)
- Multiple components
- Real network (localhost)
- End-to-end workflows

### E2E Tests

Test complete DNS server functionality.

**Location**: `pkg/dns/e2e_test.go`

**Test**: `TestE2E_FullDNSServer`
- Starts real DNS server
- Tests local records
- Tests policy engine blocking
- Tests blocklist blocking
- Tests upstream forwarding

**Characteristics:**
- Slowest (100-500ms per test)
- Full server startup/shutdown
- Real DNS protocol
- Complete workflows

### Benchmark Tests

Measure performance characteristics.

**Examples:**
```go
// pkg/blocklist/manager_test.go
func BenchmarkManager_IsBlocked(b *testing.B) {
    // Benchmark blocklist lookups
}

// pkg/cache/cache_test.go
func BenchmarkCache_Hit(b *testing.B) {
    // Benchmark cache hit performance
}
```

**Run benchmarks:**
```bash
go test -bench=BenchmarkManager_IsBlocked -benchmem ./pkg/blocklist
```

---

## Performance Testing

### Blocklist Performance

**Test Setup**: 473,873 domains from 3 major blocklists:
- OISD Big: 259,847 domains
- Hagezi Ultimate: 232,020 domains  
- StevenBlack Fake News + Gambling: 111,633 domains

**Results:**
```
Download Performance:
  Total domains:          473,873 (after deduplication)
  Download time:          725ms
  Download rate:          653,377 domains/sec

Memory Usage:
  Process RSS:            74.2 MB
  Per-domain overhead:    ~164 bytes

Lookup Performance (single-threaded):
  Blocked domain:         8ns average
  Allowed domain:         7ns average
  QPS:                    124 million

Concurrent Performance (10 goroutines):
  Avg per lookup:         2ns
  QPS:                    372 million
```

**Key Achievement**: Lock-free atomic design eliminates contention.

### Cache Performance

**Test Setup**: LRU cache with 10,000 entries

**Results:**
```
Cache Operations:
  Hit latency:            <100ns (map lookup)
  Miss latency:           ~1µs (includes upstream check)
  Set latency:            ~300ns (includes copy)
  
Throughput:
  Cache hits/sec:         10M+ per core
  Cache sets/sec:         3M+ per core

Memory:
  Per-entry overhead:     200-500 bytes
  10K entries:            2-5 MB total
```

### End-to-End Performance

**DNS Query Latency Breakdown:**
```
Component Performance:
  Blocklist lookup:       8-10ns
  Whitelist check:        ~50ns
  Cache lookup:           ~100ns
  Policy evaluation:      ~200ns
  Total overhead:         ~360ns

  Upstream forward:       4-12ms (network latency)
  
Bottleneck: Network latency dominates (99.99% of query time)
```

---

## Integration Testing

Integration tests validate component interactions.

### Test Coverage

1. **DNS + Cache**: Verifies caching works correctly with DNS queries
2. **DNS + Storage**: Validates query logging pipeline
3. **DNS + Policy**: Tests policy engine integration
4. **API + DNS**: Tests REST API management
5. **Blocklist + DNS**: Validates blocklist blocking
6. **Local Records + Cache**: Tests record caching

### Running Integration Tests

```bash
# All integration tests
go test -v ./test

# Specific integration test
go test -v ./test -run TestIntegration_DNSWithCache

# With race detector
go test -v -race ./test
```

### Integration Test Patterns

**Pattern**: Spin up real server, test real queries

```go
func TestIntegration_DNSWithCache(t *testing.T) {
    // 1. Create server with cache enabled
    cfg := &config.Config{
        Cache: config.CacheConfig{Enabled: true},
    }
    server := dns.NewServer(cfg, ...)
    
    // 2. Start server
    go server.Start(ctx)
    time.Sleep(100 * time.Millisecond)
    
    // 3. Send real DNS queries
    client := &dns.Client{}
    msg := &dns.Msg{}
    msg.SetQuestion("example.com.", dns.TypeA)
    
    // 4. Verify behavior
    resp, rtt, err := client.Exchange(msg, "127.0.0.1:5354")
    // assertions...
    
    // 5. Cleanup
    server.Shutdown(ctx)
}
```

---

## CI/CD Testing

### GitHub Actions CI

**Workflow**: `.github/workflows/ci.yml`

**Steps:**
1. Lint with golangci-lint (latest)
2. Test with coverage (`-race` flag enabled)
3. Upload coverage to Codecov
4. Build binary for Linux AMD64
5. Upload binary artifact

**Triggers:**
- Every push to `main`
- Every pull request

**Status**:  All tests passing (as of 2025-11-21)

### Race Detection

CI runs with `-race` flag to catch concurrency bugs:

```yaml
- name: Test with coverage
  run: go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
```

**Recent Fixes:**
- DNS server race in `Start()` method (fixed with mutex)
- Blocklist test race in auto-update counter (fixed with atomic.Int32)

### Linting

**Linters Enabled:**
- errcheck, govet, gofmt, goimports, gosimple
- revive, misspell, funlen, cyclop, gocognit
- And ~40 more (see `.golangci.yml`)

**Linters Disabled** (too noisy):
- dupl, ireturn, interfacebloat, nestif, maintidx
- lll, thelper, staticcheck, unparam, whitespace

---

## Writing New Tests

### Test Structure

Follow this pattern:

```go
func TestComponent_Feature_Scenario(t *testing.T) {
    // 1. Setup
    component := NewComponent(config)
    
    // 2. Execute
    result := component.DoSomething(input)
    
    // 3. Assert
    if result != expected {
        t.Errorf("Expected %v, got %v", expected, result)
    }
    
    // 4. Cleanup (if needed)
    component.Close()
}
```

### Table-Driven Tests

For testing multiple scenarios:

```go
func TestEngine_Evaluate_Patterns(t *testing.T) {
    tests := []struct {
        name     string
        rule     string
        domain   string
        expected Action
    }{
        {"simple suffix", `domain.endsWith(".com")`, "example.com", ActionAllow},
        {"regex match", `domain.matches("^ads\\.")`, "ads.google.com", ActionBlock},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

### Subtests

Use `t.Run()` for organized output:

```go
func TestDNSServer(t *testing.T) {
    t.Run("UDP", func(t *testing.T) {
        // UDP-specific tests
    })
    
    t.Run("TCP", func(t *testing.T) {
        // TCP-specific tests
    })
}
```

### Test Helpers

Mark helpers with `t.Helper()`:

```go
func setupTestServer(t *testing.T) *Server {
    t.Helper()  // Failures report caller's line
    // setup code
    return server
}
```

### Cleanup

Use `t.Cleanup()` for automatic cleanup:

```go
func TestWithServer(t *testing.T) {
    server := startServer()
    t.Cleanup(func() {
        server.Shutdown()
    })
    
    // test code - cleanup runs automatically
}
```

---

## Best Practices

### DO 

- Write tests before fixing bugs (TDD for bug fixes)
- Use table-driven tests for multiple scenarios
- Keep tests fast (<10ms per unit test)
- Use clear, descriptive test names
- Test error paths, not just happy paths
- Use subtests for organization
- Clean up resources (files, servers, goroutines)

### DON'T 

- Write flaky tests (time-dependent, random)
- Test implementation details (test behavior)
- Skip error checking in tests
- Leave goroutines running after tests
- Use `time.Sleep()` for synchronization (use channels/waitgroups)
- Ignore race detector warnings

---

## Future Testing Improvements

### Short-term
- [ ] Increase DNS handler coverage to >80%
- [ ] Add more E2E scenarios (CNAME chains, SERVFAIL, etc.)
- [ ] Benchmark tracking over time
- [ ] Mutation testing to verify test quality

### Long-term
- [ ] Chaos testing (random failures, delays)
- [ ] Load testing suite (sustained high QPS)
- [ ] Fuzz testing for parser/config
- [ ] Property-based testing for policy engine

---

## Test Maintenance

### Running Before Commits

```bash
# Quick check
go test ./...

# Full check (what CI runs)
go test -v -race -coverprofile=coverage.out ./...
golangci-lint run
```

### Fixing Failing Tests

1. **Run test in isolation**: `go test -v ./pkg/foo -run TestBar`
2. **Enable race detector**: `go test -race ./pkg/foo`
3. **Add debug output**: Use `t.Logf()` for debugging
4. **Check for flakiness**: Run 10 times: `go test -count=10 ./pkg/foo`

### Updating Tests After Changes

When modifying code:
1. Run affected package tests
2. Run integration tests if interfaces changed
3. Update test expectations if behavior changed
4. Add new tests for new functionality

---

## Conclusion

Glory-Hole has comprehensive test coverage:
- 208 tests across 13 packages
- 9,209 lines of test code
- 82.5% average coverage
- CI with race detection
- Fast test suite (<10s)

The testing strategy ensures:
-  High confidence in code changes
-  Early bug detection
-  Performance regression prevention
- 🔒 Thread-safety validation

For questions or test-related issues, refer to the tests themselves as documentation - they show exactly how each component should be used.

