// Package charon provides load balancing, request routing, and traffic management
// for the Olympus API layer. Named after the ferryman who transports souls across
// the river Styx, Charon ensures requests reach healthy backends with proper passage.
package charon

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
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

	// TLSConfig, when non-nil, enables mTLS for health-check probes. The
	// HealthChecker will create a dedicated http.Client for this shore using
	// the supplied tls.Config, allowing client-certificate authentication
	// against backends that require mutual TLS.
	TLSConfig *tls.Config
}

// NewMTLSHealthCheck constructs a HealthCheck with an mTLS configuration
// loaded from the given certificate, key, and CA files.
//
//   - certFile — path to the PEM-encoded client certificate.
//   - keyFile  — path to the PEM-encoded private key.
//   - caFile   — path to the PEM-encoded CA certificate (for server verification).
//   - path     — HTTP path to probe (e.g. "/health").
//
// The returned HealthCheck uses library-default values for Interval/Timeout;
// override those fields after construction as needed.
func NewMTLSHealthCheck(certFile, keyFile, caFile, path string) (*HealthCheck, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("charon: loading mTLS cert/key: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	if caFile != "" {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("charon: reading CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("charon: no valid CA certificates in %s", caFile)
		}
		tlsCfg.RootCAs = pool
	}

	return &HealthCheck{
		Path:      path,
		Interval:  10 * time.Second,
		Timeout:   5 * time.Second,
		Healthy:   2,
		Unhealthy: 3,
		TLSConfig: tlsCfg,
	}, nil
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

	// Session affinity key (for consistent hashing)
	// Options: "ip", "tenant", "session", "custom"
	// If empty, defaults to "ip"
	SessionAffinityKey string

	// LocalZone is the availability zone this Charon instance belongs to.
	// Used by StrategyZoneAware to prefer backends in the same zone.
	// May also be overridden per-request via the "X-Zone" header.
	LocalZone string

	// Circuit breaker settings
	CircuitBreaker CircuitBreakerConfig

	// Rate limiting configuration
	RateLimiting RateLimitConfig

	// Retry configuration
	Retry RetryConfig

	// Timeout for crossing
	CrossingTimeout time.Duration

	// StickySessionDrainTimeout controls how long sticky-session affinity
	// is preserved for a shore after it is deregistered. Defaults to 5 minutes.
	StickySessionDrainTimeout time.Duration

	// Metrics for telemetry (optional)
	Metrics interface{}

	// Maximum concurrent requests (0 = unlimited)
	MaxConcurrent int
}

// LoadBalanceStrategy determines how requests are distributed.
type LoadBalanceStrategy string

const (
	StrategyRoundRobin     LoadBalanceStrategy = "round_robin"     // Simple round-robin
	StrategyLeastConn      LoadBalanceStrategy = "least_conn"      // Least active connections
	StrategyWeighted       LoadBalanceStrategy = "weighted"        // Weighted random
	StrategyIPHash         LoadBalanceStrategy = "ip_hash"         // Simple IP hashing
	StrategyConsistentHash LoadBalanceStrategy = "consistent_hash" // Consistent hashing with virtual nodes
	StrategyZoneAware      LoadBalanceStrategy = "zone_aware"      // Prefer same zone
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
