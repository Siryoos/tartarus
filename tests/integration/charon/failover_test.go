package charon_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/charon"
)

func TestFailover(t *testing.T) {
	// Setup shores
	shore1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // Fail
	}))
	defer shore1.Close()

	shore2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // Success
	}))
	defer shore2.Close()

	// Config
	config := charon.DefaultFerryConfig()
	config.Retry.MaxRetries = 1
	config.Retry.InitialDelay = 1 * time.Millisecond
	config.Retry.RetryOn = []int{500}

	ferry, err := charon.NewBoatFerry(config)
	if err != nil {
		t.Fatalf("Failed to create ferry: %v", err)
	}
	defer ferry.Close()

	// Register shores (shore2 first so shore1 is at index 1, which RR selects first)
	// Wait, RR selects (counter % len). Counter starts at 0.
	// 1st req: counter=1. 1%2 = 1. Index 1.
	// So we want shore1 at index 1.
	// shore2, shore1.
	ferry.RegisterShore(&charon.Shore{ID: "shore2", Address: shore2.URL, Weight: 1})
	ferry.RegisterShore(&charon.Shore{ID: "shore1", Address: shore1.URL, Weight: 1})

	// Start health checker (mocked or real? Real needs time to update)
	// For this test, we rely on the retry logic which should try another shore on failure
	// regardless of health check status initially (if we force it or if it's round robin)

	// Since we use RoundRobin, we might hit shore1 first.
	// We want to ensure that if we hit shore1, we failover to shore2.

	// Make a request
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := ferry.Cross(context.Background(), req)
	if err != nil {
		t.Fatalf("Cross failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestCircuitBreaker(t *testing.T) {
	// Setup failing shore
	shore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer shore.Close()

	config := charon.DefaultFerryConfig()
	config.CircuitBreaker.Threshold = 2
	config.CircuitBreaker.Timeout = 100 * time.Millisecond
	config.Retry.MaxRetries = 0 // Disable retries to test breaker directly

	ferry, err := charon.NewBoatFerry(config)
	if err != nil {
		t.Fatalf("Failed to create ferry: %v", err)
	}
	defer ferry.Close()

	ferry.RegisterShore(&charon.Shore{ID: "shore1", Address: shore.URL})

	// Trigger failures
	ctx := context.Background()
	req := httptest.NewRequest("GET", "/", nil)

	// 1st failure
	ferry.Cross(ctx, req)
	// 2nd failure (should open breaker)
	ferry.Cross(ctx, req)

	// 3rd request should be rejected by breaker immediately
	_, err = ferry.Cross(ctx, req)
	if err == nil {
		t.Error("Expected error from circuit breaker, got nil")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should be half-open now, allow one request
	// (It will fail again, re-opening breaker)
	ferry.Cross(ctx, req)

	// Should be open again
	_, err = ferry.Cross(ctx, req)
	if err == nil {
		t.Error("Expected error from circuit breaker after re-open, got nil")
	}
}

func TestRetryBackoff(t *testing.T) {
	start := time.Now()

	shore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer shore.Close()

	config := charon.DefaultFerryConfig()
	config.Retry.MaxRetries = 2
	config.Retry.InitialDelay = 50 * time.Millisecond
	config.Retry.MaxDelay = 100 * time.Millisecond
	config.Retry.RetryOn = []int{503}

	ferry, err := charon.NewBoatFerry(config)
	if err != nil {
		t.Fatalf("Failed to create ferry: %v", err)
	}
	defer ferry.Close()

	shore2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer shore2.Close()

	shore3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer shore3.Close()

	ferry.RegisterShore(&charon.Shore{ID: "shore1", Address: shore.URL})
	ferry.RegisterShore(&charon.Shore{ID: "shore2", Address: shore2.URL})
	ferry.RegisterShore(&charon.Shore{ID: "shore3", Address: shore3.URL})

	req := httptest.NewRequest("GET", "/", nil)
	ferry.Cross(context.Background(), req)

	duration := time.Since(start)
	// Expected delay: 50ms (1st retry) + 100ms (2nd retry) = 150ms minimum
	if duration < 150*time.Millisecond {
		t.Errorf("Expected duration > 150ms, got %v", duration)
	}
}

func TestTenantRateLimiting(t *testing.T) {
	shore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer shore.Close()

	config := charon.DefaultFerryConfig()
	config.RateLimiting.Enabled = true
	config.RateLimiting.RequestsPerSecond = 10
	config.RateLimiting.Burst = 1
	config.RateLimiting.KeyFunc = "tenant"

	ferry, err := charon.NewBoatFerry(config)
	if err != nil {
		t.Fatalf("Failed to create ferry: %v", err)
	}
	defer ferry.Close()

	ferry.RegisterShore(&charon.Shore{ID: "shore1", Address: shore.URL})

	// Tenant A
	ctxA := context.WithValue(context.Background(), "tenant_id", "tenantA")
	reqA := httptest.NewRequest("GET", "/", nil)

	// Tenant B
	ctxB := context.WithValue(context.Background(), "tenant_id", "tenantB")
	reqB := httptest.NewRequest("GET", "/", nil)

	// Consume burst for Tenant A
	if _, err := ferry.Cross(ctxA, reqA); err != nil {
		t.Errorf("Tenant A request 1 failed: %v", err)
	}

	// Next request for Tenant A should fail (burst=1)
	if _, err := ferry.Cross(ctxA, reqA); err == nil {
		t.Error("Expected rate limit error for Tenant A, got nil")
	}

	// Tenant B should still succeed
	if _, err := ferry.Cross(ctxB, reqB); err != nil {
		t.Errorf("Tenant B request 1 failed: %v", err)
	}
}
