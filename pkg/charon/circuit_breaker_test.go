package charon

import (
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second, 2)

	if cb.State() != StateClosed {
		t.Errorf("Expected initial state to be Closed, got %v", cb.State())
	}

	if !cb.Allow() {
		t.Error("Expected circuit breaker to allow requests in closed state")
	}
}

func TestCircuitBreaker_OpenAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second, 2)

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != StateOpen {
		t.Errorf("Expected state to be Open after threshold failures, got %v", cb.State())
	}

	if cb.Allow() {
		t.Error("Expected circuit breaker to reject requests in open state")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond, 2)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatalf("Expected state to be Open, got %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should transition to half-open
	if !cb.Allow() {
		t.Error("Expected circuit breaker to allow request after timeout")
	}

	if cb.State() != StateHalfOpen {
		t.Errorf("Expected state to be HalfOpen after timeout, got %v", cb.State())
	}
}

func TestCircuitBreaker_CloseAfterHalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond, 2)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Transition to half-open
	cb.Allow()

	// Record successful requests
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Errorf("Expected state to be Closed after successful half-open requests, got %v", cb.State())
	}
}

func TestCircuitBreaker_ReopenOnHalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond, 2)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Transition to half-open
	cb.Allow()

	// Record failure in half-open state
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("Expected state to be Open after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Second, 2)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatalf("Expected state to be Open, got %v", cb.State())
	}

	// Reset
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("Expected state to be Closed after reset, got %v", cb.State())
	}

	if cb.Failures() != 0 {
		t.Errorf("Expected failures to be 0 after reset, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_HalfOpenRequestLimit(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond, 2)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatalf("Expected state to be Open, got %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should allow up to halfOpenRequests
	if !cb.Allow() {
		t.Error("Expected first half-open request to be allowed")
	}

	if cb.State() != StateHalfOpen {
		t.Errorf("Expected state to be HalfOpen after first request, got %v", cb.State())
	}

	if !cb.Allow() {
		t.Error("Expected second half-open request to be allowed")
	}

	// Third request should be rejected (we've reached the limit)
	if cb.Allow() {
		t.Errorf("Expected third half-open request to be rejected, state is %v", cb.State())
	}
}

func TestNoOpCircuitBreaker(t *testing.T) {
	cb := NewNoOpCircuitBreaker()

	if !cb.Allow() {
		t.Error("Expected NoOp circuit breaker to always allow requests")
	}

	cb.RecordFailure()
	if !cb.Allow() {
		t.Error("Expected NoOp circuit breaker to allow requests after failure")
	}

	if cb.State() != StateClosed {
		t.Errorf("Expected NoOp circuit breaker state to be Closed, got %v", cb.State())
	}
}
