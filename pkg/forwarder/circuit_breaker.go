// Package forwarder implements circuit breaker pattern for upstream DNS servers
package forwarder

import (
	"errors"
	"sync/atomic"
	"time"
)

var (
	// ErrCircuitOpen is returned when circuit is open (upstream unhealthy)
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrNoHealthyUpstreams is returned when all upstreams are unhealthy
	ErrNoHealthyUpstreams = errors.New("no healthy upstream servers available")
)

// CircuitState represents the state of a circuit breaker
type CircuitState int32

const (
	// StateClosed means circuit is closed and requests are forwarded normally
	StateClosed CircuitState = iota
	// StateOpen means circuit is open and requests fail fast
	StateOpen
	// StateHalfOpen means circuit is testing if upstream recovered
	StateHalfOpen
)

// String returns the string representation of the circuit state
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern for a single upstream
type CircuitBreaker struct {
	state           atomic.Int32  // Current state (StateC losed/Open/HalfOpen)
	failures        atomic.Int64  // Consecutive failure count
	successes       atomic.Int64  // Consecutive success count in half-open
	lastFailTime    atomic.Int64  // Unix nano timestamp of last failure
	lastStateChange atomic.Int64  // Unix nano timestamp of last state change
	halfOpenReqs    atomic.Int32  // Number of requests in half-open state

	// Configuration
	failureThreshold  int           // Failures before opening circuit
	successThreshold  int           // Successes to close circuit from half-open
	timeout           time.Duration // How long to wait before half-open
	halfOpenMax       int           // Max requests in half-open state
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	cb := &CircuitBreaker{
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		halfOpenMax:      3, // Allow 3 test requests in half-open
	}
	cb.state.Store(int32(StateClosed))
	cb.lastStateChange.Store(time.Now().UnixNano())
	return cb
}

// Call executes the function if circuit allows it
func (cb *CircuitBreaker) Call(fn func() error) error {
	// Check current state
	state := CircuitState(cb.state.Load())

	switch state {
	case StateOpen:
		// Check if timeout expired - transition to half-open
		timeSinceStateChange := time.Since(time.Unix(0, cb.lastStateChange.Load()))
		if timeSinceStateChange > cb.timeout {
			// Try to transition to half-open
			if cb.state.CompareAndSwap(int32(StateOpen), int32(StateHalfOpen)) {
				cb.lastStateChange.Store(time.Now().UnixNano())
				cb.successes.Store(0)
				cb.failures.Store(0)
				cb.halfOpenReqs.Store(0)
			}
		} else {
			// Circuit still open - fail fast
			return ErrCircuitOpen
		}

	case StateHalfOpen:
		// Limit concurrent requests in half-open state
		current := cb.halfOpenReqs.Add(1)
		defer cb.halfOpenReqs.Add(-1)

		if current > int32(cb.halfOpenMax) {
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

// onFailure handles a failed request
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

// onSuccess handles a successful request
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

// GetState returns the current circuit state
func (cb *CircuitBreaker) GetState() CircuitState {
	return CircuitState(cb.state.Load())
}

// IsHealthy returns true if the circuit is closed (healthy)
func (cb *CircuitBreaker) IsHealthy() bool {
	return cb.GetState() != StateOpen
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() (failures, successes int64, state CircuitState) {
	return cb.failures.Load(), cb.successes.Load(), cb.GetState()
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.state.Store(int32(StateClosed))
	cb.failures.Store(0)
	cb.successes.Store(0)
	cb.lastStateChange.Store(time.Now().UnixNano())
}
