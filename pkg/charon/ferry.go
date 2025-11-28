// Package charon provides load balancing, request routing, and traffic management
// for the Olympus API layer. Named after the ferryman who transports souls across
// the river Styx, Charon ensures requests reach healthy backends with proper passage.
package charon

import (
	"context"
	"net/http"
	"time"
)

// Ferry transports requests across the infrastructure to backend shores.
// It provides rate limiting, circuit breaking, and load balancing.
type Ferry interface {
	// Cross ferries a request to the appropriate backend shore
	Cross(ctx context.Context, req *http.Request) (*http.Response, error)

	// RegisterShore adds a backend destination
	RegisterShore(shore *Shore) error

	// DeregisterShore removes a backend
	DeregisterShore(shoreID string) error

	// Health returns ferry and shore health status
	Health(ctx context.Context) (*FerryHealth, error)

	// Close gracefully shuts down the ferry
	Close() error
}

// Shore represents a backend destination (Olympus instance).
type Shore struct {
	ID          string            // Unique identifier
	Address     string            // HTTP(S) address
	Weight      int               // Load balancing weight (higher = more traffic)
	Zone        string            // Geographic zone for zone-aware routing
	Priority    int               // Failover priority (lower = higher priority)
	HealthCheck *HealthCheck      // Health check configuration
	Metadata    map[string]string // Additional metadata
}

// HealthCheck configuration for shores.
type HealthCheck struct {
	Path      string        // HTTP path to check (e.g., "/health")
	Interval  time.Duration // Time between checks
	Timeout   time.Duration // Request timeout
	Healthy   int           // Consecutive successes to mark healthy
	Unhealthy int           // Consecutive failures to mark unhealthy
}

// FerryHealth reports overall system health.
type FerryHealth struct {
	Status       HealthStatus  // Overall ferry status
	Shores       []ShoreHealth // Health of each shore
	OpenBreakers int           // Number of open circuit breakers
	QueueDepth   int           // Pending requests (if queuing enabled)
}

// ShoreHealth reports health status of a single shore.
type ShoreHealth struct {
	ShoreID     string        // Shore identifier
	Status      HealthStatus  // Current health status
	Latency     time.Duration // Average latency
	ActiveConns int           // Active connections
	ErrorRate   float64       // Error rate (0.0-1.0)
	LastCheck   time.Time     // Last health check time
}

// HealthStatus represents the health state of a component.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"   // Fully operational
	HealthStatusDegraded  HealthStatus = "degraded"  // Partially operational
	HealthStatusUnhealthy HealthStatus = "unhealthy" // Not operational
)

// FerryConfig configures the ferry behavior.
type FerryConfig struct {
	// Load balancing strategy
	Strategy LoadBalanceStrategy

	// Circuit breaker settings
	CircuitBreaker CircuitBreakerConfig

	// Rate limiting configuration
	RateLimiting RateLimitConfig

	// Retry configuration
	Retry RetryConfig

	// Timeout for crossing
	CrossingTimeout time.Duration

	// Maximum concurrent requests (0 = unlimited)
	MaxConcurrent int
}

// LoadBalanceStrategy determines how requests are distributed.
type LoadBalanceStrategy string

const (
	StrategyRoundRobin LoadBalanceStrategy = "round_robin" // Simple round-robin
	StrategyLeastConn  LoadBalanceStrategy = "least_conn"  // Least active connections
	StrategyWeighted   LoadBalanceStrategy = "weighted"    // Weighted random
	StrategyIPHash     LoadBalanceStrategy = "ip_hash"     // Consistent hashing by IP
	StrategyZoneAware  LoadBalanceStrategy = "zone_aware"  // Prefer same zone
)

// CircuitBreakerConfig configures circuit breaker behavior.
type CircuitBreakerConfig struct {
	Enabled          bool          // Enable circuit breaker
	Threshold        int           // Failures before opening
	Timeout          time.Duration // Time before half-open
	HalfOpenRequests int           // Requests to test in half-open state
}

// RateLimitConfig configures rate limiting behavior.
type RateLimitConfig struct {
	Enabled           bool   // Enable rate limiting
	RequestsPerSecond int    // Requests per second limit
	Burst             int    // Burst capacity
	KeyFunc           string // "tenant", "ip", "identity"
	RedisAddr         string // Redis address for distributed limiting (optional)
}

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries   int           // Maximum retry attempts
	InitialDelay time.Duration // Initial delay before retry
	MaxDelay     time.Duration // Maximum delay between retries
	RetryOn      []int         // HTTP status codes to retry on
}

// DefaultFerryConfig returns sensible defaults.
func DefaultFerryConfig() *FerryConfig {
	return &FerryConfig{
		Strategy:        StrategyRoundRobin,
		CrossingTimeout: 30 * time.Second,
		MaxConcurrent:   0, // Unlimited

		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          true,
			Threshold:        5,
			Timeout:          30 * time.Second,
			HalfOpenRequests: 3,
		},

		RateLimiting: RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 100,
			Burst:             200,
			KeyFunc:           "tenant",
		},

		Retry: RetryConfig{
			MaxRetries:   2,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     2 * time.Second,
			RetryOn:      []int{502, 503, 504}, // Bad Gateway, Service Unavailable, Gateway Timeout
		},
	}
}

// DefaultHealthCheck returns sensible health check defaults.
func DefaultHealthCheck() *HealthCheck {
	return &HealthCheck{
		Path:      "/health",
		Interval:  10 * time.Second,
		Timeout:   5 * time.Second,
		Healthy:   2,
		Unhealthy: 3,
	}
}
