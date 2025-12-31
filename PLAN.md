# Performance Optimization: Upstream DNS Circuit Breaker

## Issue Classification
**Priority:** P2 - HIGH
**Category:** Performance - Resilience & Failover
**Impact:** Cascading failures during upstream outages

## Problem Statement

### Current Implementation
Location: `pkg/forwarder/forwarder.go:98`

```go
func (f *Forwarder) Forward(r *dns.Msg) (*dns.Msg, error) {
    upstream := f.selectUpstream() // Round-robin selection

    client := f.clientPool.Get().(*dns.Client)
    defer f.clientPool.Put(client)

    response, _, err := client.Exchange(r, upstream)
    if err != nil {
        // Retry with next upstream
        return f.Forward(r)
    }

    return response, nil
}
```

### Root Cause Analysis
1. **No Health Tracking:** Upstreams treated as always available
2. **Timeout Amplification:** Default 2s timeout × retries = cumulative delay
3. **Cascading Failures:** If upstream down, all queries retry and timeout
4. **No Fast Failure:** No circuit breaking to skip known-bad upstreams

### Impact Assessment
1. **Response Latency:** Queries timeout after 2-6s during upstream failures
2. **Downstream Impact:** Slow responses cause client-side timeouts
3. **Resource Waste:** CPU/memory spent on doomed requests
4. **User Experience:** DNS resolution appears "stuck" during outages

## Proposed Solution

### Circuit Breaker Pattern
Implement three states for each upstream:
- **CLOSED:** Upstream healthy, requests forwarded normally
- **OPEN:** Upstream failing, requests fail fast (no forwarding)
- **HALF_OPEN:** Testing if upstream recovered

### Architecture

```
DNS Handler → Forwarder → Circuit Breaker → Upstream Selection
                              ↓
                          Health Tracker
                              ↓
                         [CLOSED|OPEN|HALF_OPEN]
```

## Implementation

### 1. Circuit Breaker State Machine
**New File:** `pkg/forwarder/circuit_breaker.go`

```go
package forwarder

import (
    "sync"
    "sync/atomic"
    "time"
)

type CircuitState int32

const (
    StateClosed CircuitState = iota
    StateOpen
    StateHalfOpen
)

type CircuitBreaker struct {
    state         atomic.Int32
    failures      atomic.Int64
    successes     atomic.Int64
    lastFailTime  atomic.Int64
    lastStateChange atomic.Int64

    // Configuration
    failureThreshold  int           // Failures before opening circuit
    successThreshold  int           // Successes to close circuit from half-open
    timeout           time.Duration // How long to wait before half-open
    halfOpenMax       int           // Max requests in half-open state

    mu sync.RWMutex
}

func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
    cb := &CircuitBreaker{
        failureThreshold: failureThreshold,
        successThreshold: successThreshold,
        timeout:          timeout,
        halfOpenMax:      3,
    }
    cb.state.Store(int32(StateClosed))
    return cb
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    state := CircuitState(cb.state.Load())

    switch state {
    case StateOpen:
        // Check if timeout expired
        if time.Since(time.Unix(0, cb.lastStateChange.Load())) > cb.timeout {
            // Transition to half-open
            if cb.state.CompareAndSwap(int32(StateOpen), int32(StateHalfOpen)) {
                cb.lastStateChange.Store(time.Now().UnixNano())
                cb.successes.Store(0)
                cb.failures.Store(0)
            }
        } else {
            // Circuit still open - fail fast
            return ErrCircuitOpen
        }

    case StateHalfOpen:
        // Limit requests in half-open state
        if cb.failures.Load()+cb.successes.Load() >= int64(cb.halfOpenMax) {
            return ErrCircuitOpen
        }
    }

    // Execute request
    err := fn()

    if err != nil {
        cb.onFailure()
    } else {
        cb.onSuccess()
    }

    return err
}

func (cb *CircuitBreaker) onFailure() {
    failures := cb.failures.Add(1)
    cb.lastFailTime.Store(time.Now().UnixNano())

    state := CircuitState(cb.state.Load())

    switch state {
    case StateClosed:
        if failures >= int64(cb.failureThreshold) {
            // Open circuit
            if cb.state.CompareAndSwap(int32(StateClosed), int32(StateOpen)) {
                cb.lastStateChange.Store(time.Now().UnixNano())
            }
        }

    case StateHalfOpen:
        // Any failure in half-open → back to open
        if cb.state.CompareAndSwap(int32(StateHalfOpen), int32(StateOpen)) {
            cb.lastStateChange.Store(time.Now().UnixNano())
            cb.failures.Store(0)
            cb.successes.Store(0)
        }
    }
}

func (cb *CircuitBreaker) onSuccess() {
    successes := cb.successes.Add(1)
    cb.failures.Store(0) // Reset failure count on success

    state := CircuitState(cb.state.Load())

    if state == StateHalfOpen && successes >= int64(cb.successThreshold) {
        // Close circuit - upstream recovered
        if cb.state.CompareAndSwap(int32(StateHalfOpen), int32(StateClosed)) {
            cb.lastStateChange.Store(time.Now().UnixNano())
        }
    }
}

func (cb *CircuitBreaker) GetState() CircuitState {
    return CircuitState(cb.state.Load())
}

func (cb *CircuitBreaker) GetStats() (failures, successes int64, state CircuitState) {
    return cb.failures.Load(), cb.successes.Load(), cb.GetState()
}
```

### 2. Upstream Health Tracker
**File:** `pkg/forwarder/health.go` (new)

```go
package forwarder

import (
    "sync"
    "time"
)

type UpstreamHealth struct {
    breakers map[string]*CircuitBreaker
    mu       sync.RWMutex
}

func NewUpstreamHealth(upstreams []string, config CircuitBreakerConfig) *UpstreamHealth {
    uh := &UpstreamHealth{
        breakers: make(map[string]*CircuitBreaker),
    }

    for _, upstream := range upstreams {
        uh.breakers[upstream] = NewCircuitBreaker(
            config.FailureThreshold,
            config.SuccessThreshold,
            config.Timeout,
        )
    }

    return uh
}

func (uh *UpstreamHealth) IsHealthy(upstream string) bool {
    uh.mu.RLock()
    breaker, exists := uh.breakers[upstream]
    uh.mu.RUnlock()

    if !exists {
        return true // Unknown upstream - assume healthy
    }

    return breaker.GetState() != StateOpen
}

func (uh *UpstreamHealth) RecordResult(upstream string, err error) {
    uh.mu.RLock()
    breaker, exists := uh.breakers[upstream]
    uh.mu.RUnlock()

    if !exists {
        return
    }

    if err != nil {
        breaker.onFailure()
    } else {
        breaker.onSuccess()
    }
}

func (uh *UpstreamHealth) GetHealthyUpstreams(upstreams []string) []string {
    healthy := make([]string, 0, len(upstreams))

    for _, upstream := range upstreams {
        if uh.IsHealthy(upstream) {
            healthy = append(healthy, upstream)
        }
    }

    return healthy
}

func (uh *UpstreamHealth) GetStats(upstream string) (failures, successes int64, state CircuitState) {
    uh.mu.RLock()
    breaker, exists := uh.breakers[upstream]
    uh.mu.RUnlock()

    if !exists {
        return 0, 0, StateClosed
    }

    return breaker.GetStats()
}
```

### 3. Update Forwarder
**File:** `pkg/forwarder/forwarder.go`

```diff
type Forwarder struct {
    upstreams  []string
    clientPool *sync.Pool
+   health     *UpstreamHealth
    index      atomic.Uint64
    logger     *slog.Logger
}

-func (f *Forwarder) selectUpstream() string {
-   idx := f.index.Add(1) % uint64(len(f.upstreams))
-   return f.upstreams[idx]
-}
+func (f *Forwarder) selectUpstream() (string, error) {
+   // Get healthy upstreams only
+   healthy := f.health.GetHealthyUpstreams(f.upstreams)
+
+   if len(healthy) == 0 {
+       return "", ErrNoHealthyUpstreams
+   }
+
+   // Round-robin among healthy upstreams
+   idx := f.index.Add(1) % uint64(len(healthy))
+   return healthy[idx], nil
+}

func (f *Forwarder) Forward(r *dns.Msg) (*dns.Msg, error) {
-   upstream := f.selectUpstream()
+   upstream, err := f.selectUpstream()
+   if err != nil {
+       return nil, err
+   }

    client := f.clientPool.Get().(*dns.Client)
    defer f.clientPool.Put(client)

-   response, _, err := client.Exchange(r, upstream)
+   var response *dns.Msg
+   err = f.health.breakers[upstream].Call(func() error {
+       var exchangeErr error
+       response, _, exchangeErr = client.Exchange(r, upstream)
+       return exchangeErr
+   })
+
    if err != nil {
+       f.logger.Debug("Upstream forward failed",
+           "upstream", upstream,
+           "error", err,
+           "circuit_state", f.health.breakers[upstream].GetState())
-       // Retry with next upstream
-       return f.Forward(r)
+       // Try another upstream if available
+       return f.Forward(r)
    }

    return response, nil
}
```

### 4. Configuration
**File:** `pkg/config/config.go`

```diff
type Config struct {
    Forwarder struct {
        Upstreams []string `yaml:"upstreams"`
        Timeout   int      `yaml:"timeout" default:"2000"`
+       CircuitBreaker struct {
+           FailureThreshold int `yaml:"failure_threshold" default:"5"`
+           SuccessThreshold int `yaml:"success_threshold" default:"2"`
+           TimeoutSeconds   int `yaml:"timeout_seconds" default:"30"`
+       } `yaml:"circuit_breaker"`
    } `yaml:"forwarder"`
}
```

### 5. Metrics
**File:** `pkg/forwarder/metrics.go` (new)

```go
var (
    upstreamCircuitState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "gloryhole_upstream_circuit_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"upstream"},
    )

    upstreamFailures = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gloryhole_upstream_failures_total",
            Help: "Total upstream request failures",
        },
        []string{"upstream"},
    )

    upstreamCircuitOpenEvents = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gloryhole_upstream_circuit_open_total",
            Help: "Total circuit breaker open events",
        },
        []string{"upstream"},
    )
)
```

## Expected Impact

### Performance Improvements
1. **Fast Failure:**
   - Before: 2-6s timeout on dead upstream
   - After: <1ms circuit open detection
   - Impact: 2000-6000x faster failure detection

2. **Reduced Latency:**
   - Skip known-bad upstreams immediately
   - No wasted retries on failing servers
   - Queries routed to healthy upstreams only

3. **Better Failover:**
   - Automatic upstream health detection
   - Self-healing when upstream recovers
   - No manual intervention needed

4. **Resource Savings:**
   - Reduced CPU on timeout handling
   - Fewer goroutines blocked on timeouts
   - Less network traffic to dead upstreams

## Implementation Plan

### Phase 1: Circuit Breaker Core (2-3 hours)
- [ ] Implement `CircuitBreaker` state machine
- [ ] Add unit tests for state transitions
- [ ] Test failure/success thresholds
- [ ] Test timeout-based half-open transition

### Phase 2: Health Tracking (1-2 hours)
- [ ] Implement `UpstreamHealth` tracker
- [ ] Add per-upstream circuit breakers
- [ ] Test healthy upstream selection
- [ ] Add stats/metrics collection

### Phase 3: Forwarder Integration (1-2 hours)
- [ ] Update `Forwarder.Forward()` to use circuit breaker
- [ ] Update `selectUpstream()` to filter unhealthy
- [ ] Add fallback when all upstreams unhealthy
- [ ] Update error handling

### Phase 4: Configuration & Metrics (1 hour)
- [ ] Add circuit breaker config parameters
- [ ] Implement Prometheus metrics
- [ ] Add logging for circuit state changes
- [ ] Document configuration options

### Phase 5: Testing (2-3 hours)
- [ ] Unit tests: Circuit breaker state machine
- [ ] Integration tests: Upstream failover
- [ ] Chaos testing: Kill upstreams, verify behavior
- [ ] Load tests: Verify no performance regression

## Testing Plan

### Unit Tests
```go
func TestCircuitBreaker_StateTransitions(t *testing.T) {
    cb := NewCircuitBreaker(3, 2, 30*time.Second)

    // Test closed → open transition
    for i := 0; i < 3; i++ {
        cb.onFailure()
    }
    assert.Equal(t, StateOpen, cb.GetState())

    // Test open → half-open after timeout
    time.Sleep(31 * time.Second)
    cb.Call(func() error { return nil }) // Triggers transition
    assert.Equal(t, StateHalfOpen, cb.GetState())

    // Test half-open → closed on success
    cb.onSuccess()
    cb.onSuccess()
    assert.Equal(t, StateClosed, cb.GetState())
}

func TestUpstreamHealth_SelectHealthy(t *testing.T) {
    upstreams := []string{"8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"}
    health := NewUpstreamHealth(upstreams, defaultConfig)

    // Mark one upstream as unhealthy
    for i := 0; i < 5; i++ {
        health.RecordResult("8.8.8.8:53", errors.New("timeout"))
    }

    healthy := health.GetHealthyUpstreams(upstreams)
    assert.Len(t, healthy, 2)
    assert.NotContains(t, healthy, "8.8.8.8:53")
}
```

### Chaos Testing
```bash
# Start glory-hole with multiple upstreams
glory-hole --config test-config.yaml

# Kill one upstream (simulate outage)
iptables -A OUTPUT -d 8.8.8.8 -j DROP

# Verify circuit opens after 5 failures
curl http://localhost:9090/metrics | grep upstream_circuit_state

# Verify queries still succeed (routed to other upstreams)
dig @localhost -p 5353 example.com

# Restore upstream
iptables -D OUTPUT -d 8.8.8.8 -j DROP

# Verify circuit closes after recovery (30s + 2 successes)
```

## Success Metrics
- [ ] Circuit opens after configured failure threshold (default: 5)
- [ ] Circuit closes after configured success threshold in half-open (default: 2)
- [ ] Fast failure: <1ms for circuit-open upstream
- [ ] Query success rate maintained during upstream failures
- [ ] No manual intervention needed for upstream recovery
- [ ] Prometheus metrics reflect real-time circuit state

## Rollback Plan
1. Add feature flag: `enable_circuit_breaker: false`
2. If issues found, disable via config reload
3. Fallback to original round-robin with retries

## Monitoring & Alerting

### Prometheus Alerts
```yaml
groups:
- name: gloryhole_upstream_health
  rules:
  - alert: UpstreamCircuitOpen
    expr: gloryhole_upstream_circuit_state == 1
    for: 5m
    annotations:
      summary: "Upstream circuit breaker open"
      description: "{{ $labels.upstream }} circuit has been open for 5 minutes"

  - alert: AllUpstreamsUnhealthy
    expr: sum(gloryhole_upstream_circuit_state) == count(gloryhole_upstream_circuit_state)
    for: 1m
    annotations:
      summary: "All upstreams unhealthy"
      description: "All upstream DNS servers are in circuit-open state"
```

## References
- Current implementation: `pkg/forwarder/forwarder.go:98`
- Configuration: `pkg/config/config.go`

## Notes
- Default thresholds (5 failures, 2 successes, 30s timeout) tuned for typical DNS workloads
- Circuit breaker is per-upstream, not global
- Half-open state limits requests to avoid overwhelming recovering upstream
- Consider adding jitter to timeout to avoid thundering herd
