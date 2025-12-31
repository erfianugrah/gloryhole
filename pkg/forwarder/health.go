// Package forwarder implements upstream health tracking with circuit breakers
package forwarder

import (
	"sync"
	"time"
)

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled          bool          `yaml:"enabled"`           // Enable circuit breaker (default: true)
	FailureThreshold int           `yaml:"failure_threshold"` // Failures before opening (default: 5)
	SuccessThreshold int           `yaml:"success_threshold"` // Successes to close from half-open (default: 2)
	TimeoutSeconds   int           `yaml:"timeout_seconds"`   // Seconds before half-open (default: 30)
}

// DefaultCircuitBreakerConfig returns default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 5,
		SuccessThreshold: 2,
		TimeoutSeconds:   30,
	}
}

// UpstreamHealth tracks health of multiple upstreams using circuit breakers
type UpstreamHealth struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
	config   CircuitBreakerConfig
}

// NewUpstreamHealth creates a new upstream health tracker
func NewUpstreamHealth(upstreams []string, config CircuitBreakerConfig) *UpstreamHealth {
	uh := &UpstreamHealth{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}

	// Create circuit breaker for each upstream
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	for _, upstream := range upstreams {
		uh.breakers[upstream] = NewCircuitBreaker(
			config.FailureThreshold,
			config.SuccessThreshold,
			timeout,
		)
	}

	return uh
}

// IsHealthy returns true if the upstream circuit is closed (healthy)
func (uh *UpstreamHealth) IsHealthy(upstream string) bool {
	uh.mu.RLock()
	breaker, exists := uh.breakers[upstream]
	uh.mu.RUnlock()

	if !exists {
		return true // Unknown upstream - assume healthy
	}

	return breaker.IsHealthy()
}

// RecordResult records the result of an upstream request
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

// GetBreaker returns the circuit breaker for an upstream
func (uh *UpstreamHealth) GetBreaker(upstream string) *CircuitBreaker {
	uh.mu.RLock()
	defer uh.mu.RUnlock()
	return uh.breakers[upstream]
}

// GetHealthyUpstreams returns a list of healthy upstreams
func (uh *UpstreamHealth) GetHealthyUpstreams(upstreams []string) []string {
	healthy := make([]string, 0, len(upstreams))

	for _, upstream := range upstreams {
		if uh.IsHealthy(upstream) {
			healthy = append(healthy, upstream)
		}
	}

	return healthy
}

// GetStats returns statistics for an upstream
func (uh *UpstreamHealth) GetStats(upstream string) (failures, successes int64, state CircuitState) {
	uh.mu.RLock()
	breaker, exists := uh.breakers[upstream]
	uh.mu.RUnlock()

	if !exists {
		return 0, 0, StateClosed
	}

	return breaker.GetStats()
}

// GetAllStats returns statistics for all upstreams
func (uh *UpstreamHealth) GetAllStats() map[string]CircuitState {
	uh.mu.RLock()
	defer uh.mu.RUnlock()

	stats := make(map[string]CircuitState)
	for upstream, breaker := range uh.breakers {
		stats[upstream] = breaker.GetState()
	}

	return stats
}

// ResetAll resets all circuit breakers to closed state
func (uh *UpstreamHealth) ResetAll() {
	uh.mu.RLock()
	defer uh.mu.RUnlock()

	for _, breaker := range uh.breakers {
		breaker.Reset()
	}
}

// AddUpstream adds a new upstream to health tracking
func (uh *UpstreamHealth) AddUpstream(upstream string) {
	uh.mu.Lock()
	defer uh.mu.Unlock()

	if _, exists := uh.breakers[upstream]; exists {
		return
	}

	timeout := time.Duration(uh.config.TimeoutSeconds) * time.Second
	uh.breakers[upstream] = NewCircuitBreaker(
		uh.config.FailureThreshold,
		uh.config.SuccessThreshold,
		timeout,
	)
}

// RemoveUpstream removes an upstream from health tracking
func (uh *UpstreamHealth) RemoveUpstream(upstream string) {
	uh.mu.Lock()
	defer uh.mu.Unlock()

	delete(uh.breakers, upstream)
}
