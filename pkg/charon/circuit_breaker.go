package charon

import (
	"sync"
	"time"
)

// CircuitBreakerState represents the state of a circuit breaker.
type CircuitBreakerState int

const (
	StateClosed   CircuitBreakerState = iota // Normal operation, requests pass through
	StateOpen                                // Too many failures, reject requests
	StateHalfOpen                            // Testing recovery, allow limited requests
)

func (s CircuitBreakerState) String() string {
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

// CircuitBreakerInterface defines the interface for circuit breakers.
type CircuitBreakerInterface interface {
	Allow() bool
	RecordSuccess()
	RecordFailure()
	State() CircuitBreakerState
	Failures() int
	Reset()
}

// CircuitBreaker protects backends from cascading failures.
type CircuitBreaker struct {
	threshold        int           // Failures before opening
	timeout          time.Duration // Time before half-open
	halfOpenRequests int           // Requests to test in half-open

	state            CircuitBreakerState
	failures         int
	successes        int
	lastFailureTime  time.Time
	halfOpenAttempts int

	mu sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(threshold int, timeout time.Duration, halfOpenRequests int) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:        threshold,
		timeout:          timeout,
		halfOpenRequests: halfOpenRequests,
		state:            StateClosed,
	}
}

// Allow checks if a request should be allowed through the circuit breaker.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// Normal operation, allow all requests
		return true

	case StateOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastFailureTime) >= cb.timeout {
			// Transition to half-open
			cb.state = StateHalfOpen
			cb.halfOpenAttempts = 1 // Count this request
			return true
		}
		// Still open, reject request
		return false

	case StateHalfOpen:
		// Allow limited requests to test recovery
		if cb.halfOpenAttempts < cb.halfOpenRequests {
			cb.halfOpenAttempts++
			return true
		}
		// Already testing, reject additional requests
		return false

	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// Reset failure count on success
		cb.failures = 0

	case StateHalfOpen:
		cb.successes++
		// If we've had enough successful half-open requests, close the circuit
		if cb.successes >= cb.halfOpenRequests {
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
			cb.halfOpenAttempts = 0
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.threshold {
			// Open the circuit
			cb.state = StateOpen
		}

	case StateHalfOpen:
		// Any failure in half-open immediately reopens the circuit
		cb.state = StateOpen
		cb.successes = 0
		cb.halfOpenAttempts = 0
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenAttempts = 0
}

// NoOpCircuitBreaker is a circuit breaker that always allows requests.
type NoOpCircuitBreaker struct{}

// NewNoOpCircuitBreaker creates a circuit breaker that always allows requests.
func NewNoOpCircuitBreaker() *NoOpCircuitBreaker {
	return &NoOpCircuitBreaker{}
}

// Allow always returns true.
func (cb *NoOpCircuitBreaker) Allow() bool {
	return true
}

// RecordSuccess is a no-op.
func (cb *NoOpCircuitBreaker) RecordSuccess() {}

// RecordFailure is a no-op.
func (cb *NoOpCircuitBreaker) RecordFailure() {}

// State always returns StateClosed.
func (cb *NoOpCircuitBreaker) State() CircuitBreakerState {
	return StateClosed
}

// Failures always returns 0.
func (cb *NoOpCircuitBreaker) Failures() int {
	return 0
}

// Reset is a no-op.
func (cb *NoOpCircuitBreaker) Reset() {}
