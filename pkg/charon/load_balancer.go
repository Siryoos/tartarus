package charon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// BoatFerry implements the Ferry interface with load balancing, rate limiting,
// and circuit breaking capabilities.
type BoatFerry struct {
	config   *FerryConfig
	shores   []*Shore
	shoreMap map[string]*Shore

	// Components
	healthChecker *HealthChecker
	rateLimiter   RateLimiter
	breakers      map[string]CircuitBreakerInterface

	// Load balancing state
	rrCounter      uint64
	activeConns    map[string]*int32
	reverseProxies map[string]*httputil.ReverseProxy
	hashRing       *ConsistentHashRing
	telemetry      *Telemetry

	mu sync.RWMutex
}

// TelemetryCircuitBreaker wraps a CircuitBreakerInterface to record state changes.
type TelemetryCircuitBreaker struct {
	CircuitBreakerInterface
	shoreID   string
	telemetry *Telemetry
	lastState CircuitBreakerState
}

func (tcb *TelemetryCircuitBreaker) RecordSuccess() {
	tcb.CircuitBreakerInterface.RecordSuccess()
	tcb.checkState()
}

func (tcb *TelemetryCircuitBreaker) RecordFailure() {
	tcb.CircuitBreakerInterface.RecordFailure()
	tcb.checkState()
}

func (tcb *TelemetryCircuitBreaker) Reset() {
	tcb.CircuitBreakerInterface.Reset()
	tcb.checkState()
}

func (tcb *TelemetryCircuitBreaker) checkState() {
	currentState := tcb.CircuitBreakerInterface.State()
	if currentState != tcb.lastState {
		tcb.telemetry.RecordCircuitBreakerState(tcb.shoreID, currentState)
		tcb.lastState = currentState
	}
}

// NewBoatFerry creates a new ferry with the given configuration.
func NewBoatFerry(config *FerryConfig) (*BoatFerry, error) {
	if config == nil {
		config = DefaultFerryConfig()
	}

	ferry := &BoatFerry{
		config:         config,
		shores:         make([]*Shore, 0),
		shoreMap:       make(map[string]*Shore),
		breakers:       make(map[string]CircuitBreakerInterface),
		activeConns:    make(map[string]*int32),
		reverseProxies: make(map[string]*httputil.ReverseProxy),
		healthChecker:  NewHealthChecker(),
		hashRing:       NewConsistentHashRing(150),
	}

	// Initialize rate limiter
	if config.RateLimiting.Enabled {
		keyFunc := GetKeyFunc(config.RateLimiting.KeyFunc)
		ferry.rateLimiter = NewTokenBucketLimiter(
			config.RateLimiting.RequestsPerSecond,
			config.RateLimiting.Burst,
			keyFunc,
		)
	} else {
		ferry.rateLimiter = NewNoOpLimiter()
	}

	// Initialize telemetry
	if config.Metrics != nil {
		if metrics, ok := config.Metrics.(hermes.Metrics); ok {
			ferry.telemetry = &Telemetry{metrics: metrics}
		} else if metrics, ok := config.Metrics.(interface {
			IncCounter(name string, value float64, labels ...interface{})
			ObserveHistogram(name string, value float64, labels ...interface{})
			SetGauge(name string, value float64, labels ...interface{})
		}); ok {
			// Convert to hermes.Metrics interface
			// This is a bit hacky but avoids import cycles
			ferry.telemetry = &Telemetry{metrics: &metricsAdapter{raw: metrics}}
		}
	}
	if ferry.telemetry == nil {
		ferry.telemetry = &Telemetry{metrics: nil} // Will no-op
	}

	// Inject telemetry into health checker
	ferry.healthChecker.telemetry = ferry.telemetry

	return ferry, nil
}

// RegisterShore adds a backend destination.
func (f *BoatFerry) RegisterShore(shore *Shore) error {
	if shore == nil {
		return ErrInvalidConfig
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Check if shore already exists
	if _, exists := f.shoreMap[shore.ID]; exists {
		return ErrShoreAlreadyExists
	}

	// Set default health check if not provided
	if shore.HealthCheck == nil {
		shore.HealthCheck = DefaultHealthCheck()
	}

	// Set default weight if not provided
	if shore.Weight == 0 {
		shore.Weight = 1
	}

	// Parse shore address
	targetURL, err := url.Parse(shore.Address)
	if err != nil {
		return fmt.Errorf("invalid shore address: %w", err)
	}

	// Create reverse proxy for this shore
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = f.proxyErrorHandler

	// Add to collections
	f.shores = append(f.shores, shore)
	f.shoreMap[shore.ID] = shore
	f.reverseProxies[shore.ID] = proxy

	// Initialize circuit breaker
	if f.config.CircuitBreaker.Enabled {
		cb := NewCircuitBreaker(
			f.config.CircuitBreaker.Threshold,
			f.config.CircuitBreaker.Timeout,
			f.config.CircuitBreaker.HalfOpenRequests,
		)
		f.breakers[shore.ID] = &TelemetryCircuitBreaker{
			CircuitBreakerInterface: cb,
			shoreID:                 shore.ID,
			telemetry:               f.telemetry,
			lastState:               cb.State(),
		}
	} else {
		cb := NewNoOpCircuitBreaker()
		f.breakers[shore.ID] = &TelemetryCircuitBreaker{
			CircuitBreakerInterface: cb,
			shoreID:                 shore.ID,
			telemetry:               f.telemetry,
			lastState:               cb.State(),
		}
	}

	// Initialize active connections counter
	var zero int32
	f.activeConns[shore.ID] = &zero

	// Add to health checker
	f.healthChecker.AddShore(shore)

	// Add to consistent hash ring
	f.hashRing.Add(shore.ID)

	return nil
}

// DeregisterShore removes a backend.
func (f *BoatFerry) DeregisterShore(shoreID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.shoreMap[shoreID]; !exists {
		return ErrShoreNotFound
	}

	// Remove from health checker
	f.healthChecker.RemoveShore(shoreID)

	// Remove from hash ring
	f.hashRing.Remove(shoreID)

	// Remove from collections
	delete(f.shoreMap, shoreID)
	delete(f.breakers, shoreID)
	delete(f.activeConns, shoreID)
	delete(f.reverseProxies, shoreID)

	// Remove from shores slice
	for i, shore := range f.shores {
		if shore.ID == shoreID {
			f.shores = append(f.shores[:i], f.shores[i+1:]...)
			break
		}
	}

	return nil
}

// Cross ferries a request to the appropriate backend.
func (f *BoatFerry) Cross(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Apply timeout
	if f.config.CrossingTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, f.config.CrossingTimeout)
		defer cancel()
	}

	// Check rate limit (collecting the obol - payment for passage)
	// Extract key based on the rate limiter's key function
	key := ""
	if tbl, ok := f.rateLimiter.(*TokenBucketLimiter); ok {
		key = tbl.keyFunc(ctx)
	} else {
		// For NoOpLimiter or other implementations, use a default key
		key = "default"
	}

	if err := f.rateLimiter.Allow(ctx, key); err != nil {
		f.telemetry.RecordRateLimitHit(key)
		return nil, ToHTTPError(err)
	}

	// Select shore based on strategy
	shore, err := f.selectShore(ctx, req)
	if err != nil {
		return nil, ToHTTPError(err)
	}

	// Forward request with retry loop
	var resp *http.Response
	var lastErr error

	// Initial attempt + retries
	maxAttempts := 1
	if f.config.Retry.MaxRetries > 0 {
		maxAttempts += f.config.Retry.MaxRetries
	}

	// Track shores we've already tried to avoid retrying the same failing shore
	triedShores := make(map[string]bool)

	// First attempt uses the selected shore
	currentShore := shore
	triedShores[currentShore.ID] = true

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// If this is a retry (attempt > 0), we need to select a new shore if the previous one failed
		if attempt > 0 {
			// Calculate backoff
			delay := f.config.Retry.InitialDelay * time.Duration(1<<uint(attempt-1))
			if delay > f.config.Retry.MaxDelay {
				delay = f.config.Retry.MaxDelay
			}

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			// Find a new healthy shore
			nextShore, err := f.retryWithFallback(ctx, req, triedShores)
			if err != nil {
				// No more healthy shores to try
				if lastErr != nil {
					return nil, ToHTTPError(lastErr)
				}
				return nil, ToHTTPError(err)
			}
			currentShore = nextShore
			triedShores[currentShore.ID] = true
		}

		// Check circuit breaker for the current shore
		breaker := f.breakers[currentShore.ID]
		if !breaker.Allow() {
			// This shouldn't happen for the first shore if we checked before loop,
			// but could happen if state changed or for fallback shores.
			// Treat as failure and continue to next attempt
			lastErr = fmt.Errorf("circuit breaker open for shore %s", currentShore.ID)
			continue
		}

		start := time.Now()
		resp, err = f.forwardRequest(ctx, req, currentShore)
		duration := time.Since(start)

		if err != nil {
			breaker.RecordFailure()
			f.healthChecker.RecordRequest(currentShore.ID, false)
			f.telemetry.RecordRequest(currentShore.ID, false, duration)
			lastErr = err
			continue
		}

		// Check if status code warrants a retry
		shouldRetry := false
		for _, code := range f.config.Retry.RetryOn {
			if resp.StatusCode == code {
				shouldRetry = true
				break
			}
		}

		if shouldRetry {
			breaker.RecordFailure()
			f.healthChecker.RecordRequest(currentShore.ID, false)
			// Don't record telemetry failure here as it was technically a successful HTTP request, just a bad status
			// Or maybe we should? Let's stick to existing pattern but maybe we want to track these.
			// For now, just mark breaker/health.

			lastErr = fmt.Errorf("received retryable status code: %d", resp.StatusCode)

			// Close body before retrying
			if resp.Body != nil {
				resp.Body.Close()
			}
			continue
		}

		// Success!
		breaker.RecordSuccess()
		f.healthChecker.RecordRequest(currentShore.ID, true)
		f.telemetry.RecordRequest(currentShore.ID, true, duration)
		return resp, nil
	}

	// If we exhausted all attempts
	if lastErr != nil {
		return nil, ToHTTPError(lastErr)
	}
	return nil, ToHTTPError(fmt.Errorf("request failed after %d attempts", maxAttempts))
}

// selectShore chooses a backend based on the configured strategy.
func (f *BoatFerry) selectShore(ctx context.Context, req *http.Request) (*Shore, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Filter healthy shores
	healthy := make([]*Shore, 0)
	for _, shore := range f.shores {
		if f.healthChecker.IsHealthy(shore.ID) {
			healthy = append(healthy, shore)
		}
	}

	if len(healthy) == 0 {
		return nil, ErrNoHealthyShores
	}

	switch f.config.Strategy {
	case StrategyRoundRobin:
		return f.selectRoundRobin(healthy), nil

	case StrategyLeastConn:
		return f.selectLeastConn(healthy), nil

	case StrategyWeighted:
		return f.selectWeighted(healthy), nil

	case StrategyIPHash:
		return f.selectIPHash(healthy, req), nil

	case StrategyConsistentHash:
		return f.selectConsistentHash(healthy, req), nil

	default:
		return healthy[rand.Intn(len(healthy))], nil
	}
}

// selectRoundRobin selects using round-robin strategy.
func (f *BoatFerry) selectRoundRobin(shores []*Shore) *Shore {
	idx := atomic.AddUint64(&f.rrCounter, 1) % uint64(len(shores))
	return shores[idx]
}

// selectLeastConn selects the shore with fewest active connections.
func (f *BoatFerry) selectLeastConn(shores []*Shore) *Shore {
	var selected *Shore
	minConns := int32(1<<31 - 1) // Max int32

	for _, shore := range shores {
		conns := atomic.LoadInt32(f.activeConns[shore.ID])
		if conns < minConns {
			minConns = conns
			selected = shore
		}
	}

	return selected
}

// selectWeighted selects using weighted random strategy.
func (f *BoatFerry) selectWeighted(shores []*Shore) *Shore {
	totalWeight := 0
	for _, shore := range shores {
		totalWeight += shore.Weight
	}

	r := rand.Intn(totalWeight)
	for _, shore := range shores {
		r -= shore.Weight
		if r < 0 {
			return shore
		}
	}

	return shores[0]
}

// selectIPHash selects using simple IP hashing (legacy).
func (f *BoatFerry) selectIPHash(shores []*Shore, req *http.Request) *Shore {
	// Extract IP from request
	ip := req.RemoteAddr
	if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = forwarded
	}

	// Hash IP to select shore
	hash := sha256.Sum256([]byte(ip))
	idx := binary.BigEndian.Uint64(hash[:8]) % uint64(len(shores))
	return shores[idx]
}

// selectConsistentHash uses consistent hashing ring for sticky sessions.
func (f *BoatFerry) selectConsistentHash(shores []*Shore, req *http.Request) *Shore {
	// Extract session key based on configuration
	key := f.extractSessionKey(req)

	// Get primary shore from hash ring
	primaryID := f.hashRing.Get(key)
	if primaryID == "" {
		// Ring is empty, fallback to random
		return shores[rand.Intn(len(shores))]
	}

	// Check if primary is healthy
	for _, shore := range shores {
		if shore.ID == primaryID {
			return shore
		}
	}

	// Primary is not healthy, get fallback shores
	fallbacks := f.hashRing.GetN(key, 3)
	for _, fallbackID := range fallbacks {
		for _, shore := range shores {
			if shore.ID == fallbackID {
				return shore
			}
		}
	}

	// No consistent hash match found (shouldn't happen), use first healthy
	return shores[0]
}

// extractSessionKey extracts the session affinity key from the request.
func (f *BoatFerry) extractSessionKey(req *http.Request) string {
	affinityKey := f.config.SessionAffinityKey
	if affinityKey == "" {
		affinityKey = "ip" // Default
	}

	switch affinityKey {
	case "tenant":
		// Extract tenant ID from context or header
		if tenantID := req.Context().Value("tenant_id"); tenantID != nil {
			return fmt.Sprintf("tenant:%v", tenantID)
		}
		if tenantID := req.Header.Get("X-Tenant-ID"); tenantID != "" {
			return fmt.Sprintf("tenant:%s", tenantID)
		}

	case "session":
		// Extract session ID from cookie or header
		if cookie, err := req.Cookie("session_id"); err == nil {
			return fmt.Sprintf("session:%s", cookie.Value)
		}
		if sessionID := req.Header.Get("X-Session-ID"); sessionID != "" {
			return fmt.Sprintf("session:%s", sessionID)
		}

	case "custom":
		// Extract from custom header
		if customKey := req.Header.Get("X-Affinity-Key"); customKey != "" {
			return customKey
		}
	}

	// Default to IP
	ip := req.RemoteAddr
	if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = forwarded
	}
	return fmt.Sprintf("ip:%s", ip)
}

// forwardRequest forwards the request to the selected shore.
func (f *BoatFerry) forwardRequest(ctx context.Context, req *http.Request, shore *Shore) (*http.Response, error) {
	// Increment active connections
	newCount := atomic.AddInt32(f.activeConns[shore.ID], 1)
	f.telemetry.RecordActiveConnections(shore.ID, int(newCount))
	defer func() {
		newCount := atomic.AddInt32(f.activeConns[shore.ID], -1)
		f.telemetry.RecordActiveConnections(shore.ID, int(newCount))
	}()

	// Get reverse proxy for this shore
	proxy := f.reverseProxies[shore.ID]

	// Create a response writer to capture the response
	recorder := &responseRecorder{
		header: make(http.Header),
	}

	// Forward request
	proxy.ServeHTTP(recorder, req.WithContext(ctx))

	// Build response
	resp := &http.Response{
		StatusCode: recorder.statusCode,
		Header:     recorder.header,
		Body:       io.NopCloser(recorder.body),
		Request:    req,
	}

	if recorder.statusCode == 0 {
		resp.StatusCode = http.StatusOK
	}

	return resp, nil
}

// proxyErrorHandler handles errors from the reverse proxy.
func (f *BoatFerry) proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

// retryWithFallback tries to find an alternative healthy shore.
func (f *BoatFerry) retryWithFallback(ctx context.Context, req *http.Request, triedShores map[string]bool) (*Shore, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Find healthy shores excluding the ones we've already tried
	for _, shore := range f.shores {
		if triedShores[shore.ID] {
			continue
		}
		if !f.healthChecker.IsHealthy(shore.ID) {
			continue
		}
		if !f.breakers[shore.ID].Allow() {
			continue
		}

		// Found a candidate
		return shore, nil
	}

	return nil, ErrNoHealthyShores
}

// Health returns ferry and shore health status.
func (f *BoatFerry) Health(ctx context.Context) (*FerryHealth, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	shoreHealth := f.healthChecker.GetAllShoreHealth()

	// Count open breakers
	openBreakers := 0
	for _, breaker := range f.breakers {
		if breaker.State() == StateOpen {
			openBreakers++
		}
	}

	// Determine overall status
	status := HealthStatusHealthy
	healthyCount := 0
	for _, sh := range shoreHealth {
		if sh.Status == HealthStatusHealthy {
			healthyCount++
		}
	}

	if healthyCount == 0 {
		status = HealthStatusUnhealthy
	} else if healthyCount < len(shoreHealth) {
		status = HealthStatusDegraded
	}

	// Add active connections to shore health
	for i := range shoreHealth {
		shoreHealth[i].ActiveConns = int(atomic.LoadInt32(f.activeConns[shoreHealth[i].ShoreID]))
	}

	return &FerryHealth{
		Status:       status,
		Shores:       shoreHealth,
		OpenBreakers: openBreakers,
	}, nil
}

// Start starts the ferry (health checking, etc.).
func (f *BoatFerry) Start(ctx context.Context) {
	f.healthChecker.Start(ctx)
}

// Close gracefully shuts down the ferry.
func (f *BoatFerry) Close() error {
	f.healthChecker.Stop()
	return f.rateLimiter.Close()
}

// responseRecorder captures HTTP responses.
type responseRecorder struct {
	statusCode int
	header     http.Header
	body       *bytes.Buffer
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.body == nil {
		r.body = &bytes.Buffer{}
	}
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}
