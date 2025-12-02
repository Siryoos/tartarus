package charon

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthChecker performs periodic health checks on shores.
type HealthChecker struct {
	shores map[string]*shoreHealthState
	client *http.Client
	mu     sync.RWMutex

	stopChan chan struct{}
	doneChan chan struct{}

	telemetry *Telemetry
}

// shoreHealthState tracks health state for a single shore.
type shoreHealthState struct {
	shore              *Shore
	status             HealthStatus
	consecutiveSuccess int
	consecutiveFailure int
	lastCheck          time.Time
	latency            time.Duration
	errorRate          float64
	totalRequests      int64
	failedRequests     int64
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		shores: make(map[string]*shoreHealthState),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
}

// AddShore adds a shore to be monitored.
func (hc *HealthChecker) AddShore(shore *Shore) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.shores[shore.ID] = &shoreHealthState{
		shore:  shore,
		status: HealthStatusHealthy, // Assume healthy initially
	}
}

// RemoveShore removes a shore from monitoring.
func (hc *HealthChecker) RemoveShore(shoreID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	delete(hc.shores, shoreID)
}

// Start begins periodic health checks.
func (hc *HealthChecker) Start(ctx context.Context) {
	go hc.run(ctx)
}

// Stop stops health checking.
func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	<-hc.doneChan
}

// run performs periodic health checks.
func (hc *HealthChecker) run(ctx context.Context) {
	defer close(hc.doneChan)

	// Stagger initial checks to avoid thundering herd
	hc.mu.RLock()
	shores := make([]*shoreHealthState, 0, len(hc.shores))
	for _, state := range hc.shores {
		shores = append(shores, state)
	}
	hc.mu.RUnlock()

	// Start a goroutine for each shore
	var wg sync.WaitGroup
	for _, state := range shores {
		wg.Add(1)
		go func(state *shoreHealthState) {
			defer wg.Done()
			hc.checkShore(ctx, state)
		}(state)
	}

	wg.Wait()
}

// checkShore performs health checks for a single shore.
func (hc *HealthChecker) checkShore(ctx context.Context, state *shoreHealthState) {
	if state.shore.HealthCheck == nil {
		return
	}

	interval := state.shore.HealthCheck.Interval
	if interval == 0 {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.performCheck(ctx, state)

		case <-hc.stopChan:
			return

		case <-ctx.Done():
			return
		}
	}
}

// performCheck executes a single health check.
func (hc *HealthChecker) performCheck(ctx context.Context, state *shoreHealthState) {
	healthCheck := state.shore.HealthCheck
	if healthCheck == nil {
		return
	}

	// Build health check URL
	url := fmt.Sprintf("%s%s", state.shore.Address, healthCheck.Path)

	// Create request with timeout
	checkCtx, cancel := context.WithTimeout(ctx, healthCheck.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, url, nil)
	if err != nil {
		hc.recordFailure(state)
		return
	}

	// Perform check and measure latency
	start := time.Now()
	resp, err := hc.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		hc.recordFailure(state)
		return
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		hc.recordSuccess(state, latency)
		if hc.telemetry != nil {
			hc.telemetry.RecordHealthCheck(state.shore.ID, true, latency)
		}
	} else {
		hc.recordFailure(state)
		if hc.telemetry != nil {
			hc.telemetry.RecordHealthCheck(state.shore.ID, false, latency)
		}
	}
}

// recordSuccess records a successful health check.
func (hc *HealthChecker) recordSuccess(state *shoreHealthState, latency time.Duration) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	state.consecutiveSuccess++
	state.consecutiveFailure = 0
	state.lastCheck = time.Now()
	state.latency = latency

	// Update status if needed
	if state.shore.HealthCheck != nil &&
		state.consecutiveSuccess >= state.shore.HealthCheck.Healthy {
		if state.status != HealthStatusHealthy {
			state.status = HealthStatusHealthy
			if hc.telemetry != nil {
				hc.telemetry.RecordShoreHealth(state.shore.ID, HealthStatusHealthy)
			}
		}
	}
}

// recordFailure records a failed health check.
func (hc *HealthChecker) recordFailure(state *shoreHealthState) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	state.consecutiveFailure++
	state.consecutiveSuccess = 0
	state.lastCheck = time.Now()

	// Update status if needed
	if state.shore.HealthCheck != nil &&
		state.consecutiveFailure >= state.shore.HealthCheck.Unhealthy {
		if state.status != HealthStatusUnhealthy {
			state.status = HealthStatusUnhealthy
			if hc.telemetry != nil {
				hc.telemetry.RecordShoreHealth(state.shore.ID, HealthStatusUnhealthy)
			}
		}
	}
}

// GetShoreHealth returns the health status of a shore.
func (hc *HealthChecker) GetShoreHealth(shoreID string) *ShoreHealth {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	state, exists := hc.shores[shoreID]
	if !exists {
		return nil
	}

	return &ShoreHealth{
		ShoreID:   shoreID,
		Status:    state.status,
		Latency:   state.latency,
		ErrorRate: state.errorRate,
		LastCheck: state.lastCheck,
	}
}

// GetAllShoreHealth returns health status for all shores.
func (hc *HealthChecker) GetAllShoreHealth() []ShoreHealth {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	health := make([]ShoreHealth, 0, len(hc.shores))
	for _, state := range hc.shores {
		health = append(health, ShoreHealth{
			ShoreID:   state.shore.ID,
			Status:    state.status,
			Latency:   state.latency,
			ErrorRate: state.errorRate,
			LastCheck: state.lastCheck,
		})
	}

	return health
}

// IsHealthy returns true if the shore is healthy.
func (hc *HealthChecker) IsHealthy(shoreID string) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	state, exists := hc.shores[shoreID]
	if !exists {
		return false
	}

	return state.status == HealthStatusHealthy
}

// RecordRequest records a request for error rate tracking.
func (hc *HealthChecker) RecordRequest(shoreID string, success bool) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	state, exists := hc.shores[shoreID]
	if !exists {
		return
	}

	state.totalRequests++
	if !success {
		state.failedRequests++
	}

	// Calculate error rate (exponential moving average)
	if state.totalRequests > 0 {
		state.errorRate = float64(state.failedRequests) / float64(state.totalRequests)
	}
}
