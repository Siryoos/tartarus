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

	mu sync.RWMutex
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
		f.breakers[shore.ID] = NewCircuitBreaker(
			f.config.CircuitBreaker.Threshold,
			f.config.CircuitBreaker.Timeout,
			f.config.CircuitBreaker.HalfOpenRequests,
		)
	} else {
		f.breakers[shore.ID] = NewNoOpCircuitBreaker()
	}

	// Initialize active connections counter
	var zero int32
	f.activeConns[shore.ID] = &zero

	// Add to health checker
	f.healthChecker.AddShore(shore)

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
	key := f.rateLimiter.(*TokenBucketLimiter).keyFunc(ctx)
	if err := f.rateLimiter.Allow(ctx, key); err != nil {
		return nil, ToHTTPError(err)
	}

	// Select shore based on strategy
	shore, err := f.selectShore(ctx, req)
	if err != nil {
		return nil, ToHTTPError(err)
	}

	// Check circuit breaker
	breaker := f.breakers[shore.ID]
	if !breaker.Allow() {
		// Try to find an alternative shore
		return f.retryWithFallback(ctx, req, shore.ID)
	}

	// Forward request
	resp, err := f.forwardRequest(ctx, req, shore)
	if err != nil {
		breaker.RecordFailure()
		f.healthChecker.RecordRequest(shore.ID, false)

		// Retry if configured
		if f.config.Retry.MaxRetries > 0 {
			return f.retryRequest(ctx, req, shore.ID, 0)
		}
		return nil, ToHTTPError(err)
	}

	breaker.RecordSuccess()
	f.healthChecker.RecordRequest(shore.ID, true)

	return resp, nil
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

// selectIPHash selects using consistent hashing by IP.
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

// forwardRequest forwards the request to the selected shore.
func (f *BoatFerry) forwardRequest(ctx context.Context, req *http.Request, shore *Shore) (*http.Response, error) {
	// Increment active connections
	atomic.AddInt32(f.activeConns[shore.ID], 1)
	defer atomic.AddInt32(f.activeConns[shore.ID], -1)

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

// retryRequest retries a failed request with exponential backoff.
func (f *BoatFerry) retryRequest(ctx context.Context, req *http.Request, excludeShoreID string, attempt int) (*http.Response, error) {
	if attempt >= f.config.Retry.MaxRetries {
		return nil, fmt.Errorf("max retries exceeded")
	}

	// Calculate delay with exponential backoff
	delay := f.config.Retry.InitialDelay * time.Duration(1<<uint(attempt))
	if delay > f.config.Retry.MaxDelay {
		delay = f.config.Retry.MaxDelay
	}

	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Try again with a different shore
	return f.retryWithFallback(ctx, req, excludeShoreID)
}

// retryWithFallback tries to forward to an alternative shore.
func (f *BoatFerry) retryWithFallback(ctx context.Context, req *http.Request, excludeShoreID string) (*http.Response, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Find healthy shores excluding the failed one
	for _, shore := range f.shores {
		if shore.ID == excludeShoreID {
			continue
		}
		if !f.healthChecker.IsHealthy(shore.ID) {
			continue
		}
		if !f.breakers[shore.ID].Allow() {
			continue
		}

		// Try this shore
		resp, err := f.forwardRequest(ctx, req, shore)
		if err == nil {
			f.breakers[shore.ID].RecordSuccess()
			f.healthChecker.RecordRequest(shore.ID, true)
			return resp, nil
		}

		f.breakers[shore.ID].RecordFailure()
		f.healthChecker.RecordRequest(shore.ID, false)
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
