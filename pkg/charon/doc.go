// Package charon implements the request ferry and load balancer — the Ferryman
// of the Underworld who transports every API request to a healthy backend shore.
//
// Charon sits in front of the Olympus API tier and provides:
//   - Five load-balancing strategies (round-robin, least-connections, random,
//     weighted, consistent-hash).
//   - Token-bucket rate limiting per client key.
//   - A three-state circuit breaker (closed → open → half-open) per backend.
//   - Active health checking with configurable intervals and thresholds.
//   - Prometheus telemetry for latency, error rates, and circuit-breaker states.
//
// # Core Concepts
//
// A [Ferry] transports HTTP requests across a pool of [Shore] backends. The
// caller registers shores and then calls [Ferry.Cross] for each request.
//
// A [RateLimiter] gates requests per-key before they reach any backend.
//
// A [CircuitBreakerInterface] wraps each shore and opens when failure
// thresholds are exceeded, preventing the caller from hammering a degraded
// backend.
//
// # Basic Usage
//
//	cfg := &charon.FerryConfig{
//	    Strategy:           charon.StrategyRoundRobin,
//	    RateLimit:          1000, // req/s
//	    CircuitBreakerCfg: charon.CircuitBreakerConfig{
//	        Threshold:        5,
//	        Timeout:          10 * time.Second,
//	        HalfOpenRequests: 2,
//	    },
//	}
//	ferry, err := charon.NewBoatFerry(cfg, logger)
//	
//	ferry.RegisterShore(&charon.Shore{
//	    ID:      "olympus-1",
//	    Address: "http://olympus-1:8080",
//	    Weight:  10,
//	})
//
//	// In your HTTP handler or middleware:
//	resp, err := ferry.Cross(ctx, req)
//
// # Known Technical Debt
//
//   - Consistent-hash affinity is not sticky across shore deregistrations;
//     removing a shore causes a full re-hash and breaks in-flight sessions.
//
//   - Rate limiting is purely in-memory ([TokenBucketLimiter]). In a
//     multi-instance deployment, limits are per-process. A Redis-backed
//     distributed rate limiter is required for true global enforcement.
//
//   - The health checker does not support TLS client certificates for backend
//     probes. This is a gap when backends require mTLS.
//
//   - WebSocket and long-lived streaming connections are not transparently
//     proxied. Only standard HTTP request-response is supported.
//
//   - Zone-aware routing (using [Shore.Zone]) is parsed but not yet used in
//     any selection strategy; the implementation prefers same-zone backends
//     but falls back globally without a configurable policy.
package charon
