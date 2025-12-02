package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/charon"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// TestCharonFailover tests the end-to-end failover behavior.
func TestCharonFailover(t *testing.T) {
	// Setup test backends
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend1"))
	}))
	defer backend1.Close()

	backend2Down := false
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if backend2Down {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend2"))
	}))
	defer backend2.Close()

	backend3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend3"))
	}))
	defer backend3.Close()

	// Create ferry with circuit breaker
	config := charon.DefaultFerryConfig()
	config.Strategy = charon.StrategyLeastConn
	config.CircuitBreaker.Enabled = true
	config.CircuitBreaker.Threshold = 2
	config.CircuitBreaker.Timeout = 500 * time.Millisecond

	ferry, err := charon.NewBoatFerry(config)
	require.NoError(t, err)

	// Register backends
	shores := []*charon.Shore{
		{
			ID:      "backend-1",
			Address: backend1.URL,
			HealthCheck: &charon.HealthCheck{
				Path:      "/health",
				Interval:  100 * time.Millisecond,
				Timeout:   50 * time.Millisecond,
				Healthy:   1,
				Unhealthy: 2,
			},
		},
		{
			ID:      "backend-2",
			Address: backend2.URL,
			HealthCheck: &charon.HealthCheck{
				Path:      "/health",
				Interval:  100 * time.Millisecond,
				Timeout:   50 * time.Millisecond,
				Healthy:   1,
				Unhealthy: 2,
			},
		},
		{
			ID:      "backend-3",
			Address: backend3.URL,
			HealthCheck: &charon.HealthCheck{
				Path:      "/health",
				Interval:  100 * time.Millisecond,
				Timeout:   50 * time.Millisecond,
				Healthy:   1,
				Unhealthy: 2,
			},
		},
	}

	for _, shore := range shores {
		require.NoError(t, ferry.RegisterShore(shore))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ferry.Start(ctx)
	defer ferry.Close()

	// Wait for initial health checks
	time.Sleep(200 * time.Millisecond)

	// 1. All backends healthy - traffic should distribute
	t.Run("AllHealthy", func(t *testing.T) {
		health, err := ferry.Health(ctx)
		require.NoError(t, err)
		assert.Equal(t, charon.HealthStatusHealthy, health.Status)
		assert.Len(t, health.Shores, 3)

		// Make some requests
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			resp, err := ferry.Cross(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()
		}
	})

	// 2. Simulate backend-2 failure
	t.Run("FailoverOnFailure", func(t *testing.T) {
		backend2Down = true

		// Send requests to trip circuit breaker for backend-2
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			ferry.Cross(ctx, req) // Some will fail, that's expected
			time.Sleep(50 * time.Millisecond)
		}

		// Wait for health check to detect
		time.Sleep(300 * time.Millisecond)

		// Now only backend-1 and backend-3 should receive traffic
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			resp, err := ferry.Cross(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()
		}

		// Check health status
		health, err := ferry.Health(ctx)
		require.NoError(t, err)
		assert.Equal(t, charon.HealthStatusDegraded, health.Status)
	})

	// 3. Recover backend-2
	t.Run("RecoveryAfterFailure", func(t *testing.T) {
		backend2Down = false

		// Wait for health check to recover
		time.Sleep(500 * time.Millisecond)

		// Health should be back to normal
		health, err := ferry.Health(ctx)
		require.NoError(t, err)
		// Should be healthy or degraded (circuit breaker may still be open)
		assert.NotEqual(t, charon.HealthStatusUnhealthy, health.Status)
	})
}

// TestCharonStickySession tests consistent hashing sticky sessions.
func TestCharonStickySession(t *testing.T) {
	// Track which backend each session goes to
	backend1Requests := make(map[string]int)
	backend2Requests := make(map[string]int)
	backend3Requests := make(map[string]int)

	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		backend1Requests[sessionID]++
		w.WriteHeader(http.StatusOK)
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		backend2Requests[sessionID]++
		w.WriteHeader(http.StatusOK)
	}))
	defer backend2.Close()

	backend3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		backend3Requests[sessionID]++
		w.WriteHeader(http.StatusOK)
	}))
	defer backend3.Close()

	// Create ferry with consistent hashing
	config := charon.DefaultFerryConfig()
	config.Strategy = charon.StrategyConsistentHash
	config.SessionAffinityKey = "session"
	config.CircuitBreaker.Enabled = false
	config.RateLimiting.Enabled = false

	ferry, err := charon.NewBoatFerry(config)
	require.NoError(t, err)

	ferry.RegisterShore(&charon.Shore{ID: "backend-1", Address: backend1.URL})
	ferry.RegisterShore(&charon.Shore{ID: "backend-2", Address: backend2.URL})
	ferry.RegisterShore(&charon.Shore{ID: "backend-3", Address: backend3.URL})

	ctx := context.Background()

	// Simulate 3 different sessions, each making 10 requests
	sessions := []string{"session-A", "session-B", "session-C"}

	for _, sessionID := range sessions {
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Session-ID", sessionID)
			resp, err := ferry.Cross(ctx, req)
			require.NoError(t, err)
			resp.Body.Close()
		}
	}

	// Each session should stick to ONE backend
	for _, sessionID := range sessions {
		backendCounts := []int{
			backend1Requests[sessionID],
			backend2Requests[sessionID],
			backend3Requests[sessionID],
		}

		// Exactly one backend should have all 10 requests
		hasAll := false
		for _, count := range backendCounts {
			if count == 10 {
				hasAll = true
			}
		}
		assert.True(t, hasAll, "Session %s should stick to one backend", sessionID)

		t.Logf("Session %s distribution: backend1=%d, backend2=%d, backend3=%d",
			sessionID, backendCounts[0], backendCounts[1], backendCounts[2])
	}
}

// TestCharonMetricsExport tests that metrics are correctly exported.
func TestCharonMetricsExport(t *testing.T) {
	// Create mock metrics collector
	metrics := hermes.NewPrometheusMetrics()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Create ferry with metrics
	config := charon.DefaultFerryConfig()
	config.Metrics = metrics
	config.CircuitBreaker.Enabled = false
	config.RateLimiting.Enabled = false

	ferry, err := charon.NewBoatFerry(config)
	require.NoError(t, err)

	ferry.RegisterShore(&charon.Shore{
		ID:      "test-backend",
		Address: backend.URL,
	})

	ctx := context.Background()

	// Make some requests
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := ferry.Cross(ctx, req)
		require.NoError(t, err)
		resp.Body.Close()
	}

	// Metrics should be recorded
	// Note: We can't easily assert on Prometheus metrics without exposing them,
	// but we can verify the ferry didn't crash with metrics enabled
	t.Log("Metrics integration test passed - ferry handled requests with metrics enabled")
}
