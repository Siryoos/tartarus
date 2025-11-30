package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/charon"
)

func TestCharonTrafficFerry(t *testing.T) {
	// 1. Setup Mock Backends (Shores)
	backend1Count := int32(0)
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		atomic.AddInt32(&backend1Count, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend1"))
	}))
	defer backend1.Close()

	backend2Count := int32(0)
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		atomic.AddInt32(&backend2Count, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend2"))
	}))
	defer backend2.Close()

	// 2. Configure Charon
	config := charon.DefaultFerryConfig()
	config.Strategy = charon.StrategyRoundRobin
	// Fast health checks for testing
	config.CircuitBreaker.Timeout = 100 * time.Millisecond
	config.CircuitBreaker.Threshold = 2

	ferry, err := charon.NewBoatFerry(config)
	require.NoError(t, err)

	// Register Shores
	err = ferry.RegisterShore(&charon.Shore{
		ID:      "shore-1",
		Address: backend1.URL,
		HealthCheck: &charon.HealthCheck{
			Path:      "/health",
			Interval:  50 * time.Millisecond,
			Timeout:   50 * time.Millisecond,
			Healthy:   1,
			Unhealthy: 1,
		},
	})
	require.NoError(t, err)

	err = ferry.RegisterShore(&charon.Shore{
		ID:      "shore-2",
		Address: backend2.URL,
		HealthCheck: &charon.HealthCheck{
			Path:      "/health",
			Interval:  50 * time.Millisecond,
			Timeout:   50 * time.Millisecond,
			Healthy:   1,
			Unhealthy: 1,
		},
	})
	require.NoError(t, err)

	// Start Ferry
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ferry.Start(ctx)
	defer ferry.Close()

	// Wait for initial health checks
	time.Sleep(200 * time.Millisecond)

	// 3. Test Load Balancing (Round Robin)
	t.Run("LoadBalancing", func(t *testing.T) {
		// Reset counters
		atomic.StoreInt32(&backend1Count, 0)
		atomic.StoreInt32(&backend2Count, 0)

		// Send 10 requests
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			resp, err := ferry.Cross(ctx, req)
			require.NoError(t, err)
			resp.Body.Close()
		}

		// Should be roughly equal
		assert.Equal(t, int32(5), atomic.LoadInt32(&backend1Count))
		assert.Equal(t, int32(5), atomic.LoadInt32(&backend2Count))
	})

	// 4. Test Circuit Breaking
	t.Run("CircuitBreaking", func(t *testing.T) {
		// Close backend 2 to simulate failure
		backend2.Close()

		// Wait for health check to detect failure or circuit breaker to trip
		// We need to send requests to trip the breaker
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			ferry.Cross(ctx, req) // Ignore errors
		}

		// Now traffic should only go to backend 1
		atomic.StoreInt32(&backend1Count, 0)

		successCount := 0
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			_, err := ferry.Cross(ctx, req)
			if err == nil {
				successCount++
			}
		}

		// All successful requests should be from backend 1
		assert.Equal(t, int32(successCount), atomic.LoadInt32(&backend1Count))
		// We expect some successes (from backend 1)
		assert.Greater(t, successCount, 0)
	})

	// 5. Test Rate Limiting
	t.Run("RateLimiting", func(t *testing.T) {
		// Create a new ferry with strict rate limits
		rlConfig := charon.DefaultFerryConfig()
		rlConfig.RateLimiting.Enabled = true
		rlConfig.RateLimiting.RequestsPerSecond = 10
		rlConfig.RateLimiting.Burst = 10
		rlConfig.RateLimiting.KeyFunc = "tenant"

		rlFerry, err := charon.NewBoatFerry(rlConfig)
		require.NoError(t, err)

		// Register a dummy shore
		rlFerry.RegisterShore(&charon.Shore{
			ID:      "shore-1",
			Address: backend1.URL,
		})

		// Send 20 requests rapidly
		var wg sync.WaitGroup
		success := int32(0)
		failures := int32(0)

		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req := httptest.NewRequest("GET", "/", nil)
				// Set tenant ID
				ctxWithTenant := context.WithValue(context.Background(), "tenant_id", "tenant-A")

				_, err := rlFerry.Cross(ctxWithTenant, req)
				if err == nil {
					atomic.AddInt32(&success, 1)
				} else {
					atomic.AddInt32(&failures, 1)
				}
			}()
		}
		wg.Wait()

		// Should allow around 10 (burst) and reject the rest
		assert.True(t, atomic.LoadInt32(&success) <= 12, "Should not exceed burst significantly")
		assert.True(t, atomic.LoadInt32(&failures) >= 8, "Should reject excess requests")
	})
}
