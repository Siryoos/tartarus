// Package charon implements the request ferry and load balancer — the Ferryman
// of the Underworld who transports every API request to a healthy backend shore.
//
// Charon sits in front of the Olympus API tier and provides:
//   - Five load-balancing strategies: round-robin, least-connections, weighted,
//     IP-hash, consistent-hash with sticky-session draining, and zone-aware routing.
//   - Per-process token-bucket rate limiting ([TokenBucketLimiter]) and a
//     globally-consistent sliding-window limiter backed by Redis
//     ([RedisRateLimiter]). The Redis limiter is selected automatically when
//     [RateLimitConfig.RedisAddr] is non-empty.
//   - A three-state circuit breaker (closed → open → half-open) per backend.
//   - Active health checking with configurable intervals and thresholds. Backends
//     that require mutual TLS can supply a [HealthCheck.TLSConfig] so the prober
//     presents a client certificate on every probe.
//   - Transparent WebSocket upgrade proxying via a bidirectional gorilla tunnel.
//   - Server-Sent Events (SSE) and long-polling streaming via incremental
//     io.Copy with flushing.
//   - Zone-aware routing ([StrategyZoneAware]) that prefers same-zone backends
//     and falls back globally when no local backends are healthy.
//   - Prometheus telemetry for latency, error rates, and circuit-breaker states.
//
// # Core Concepts
//
// A [Ferry] transports HTTP requests across a pool of [Shore] backends. The
// caller registers shores and then calls [Ferry.Cross] for each request.
//
// A [RateLimiter] gates requests per-key before they reach any backend.
// Use [NewRedisRateLimiter] for multi-instance deployments.
//
// A [CircuitBreakerInterface] wraps each shore and opens when failure
// thresholds are exceeded, preventing the caller from hammering a degraded
// backend.
//
// A [StickySessionTable] is maintained internally by [BoatFerry]. It pins
// session-key → shore-ID affinity established via consistent-hash routing. When
// a shore is deregistered, its entries transition to a draining state and are
// honoured for a configurable [FerryConfig.StickySessionDrainTimeout] before
// being evicted, ensuring in-flight sessions complete on their original backend.
//
// # Basic Usage
//
//	cfg := &charon.FerryConfig{
//	    Strategy:  charon.StrategyConsistentHash,
//	    LocalZone: "us-east-1a",
//	    RateLimiting: charon.RateLimitConfig{
//	        Enabled:           true,
//	        RequestsPerSecond: 1000,
//	        Burst:             2000,
//	        RedisAddr:         "redis:6379", // omit for in-process limiting
//	    },
//	    CircuitBreaker: charon.CircuitBreakerConfig{
//	        Enabled:          true,
//	        Threshold:        5,
//	        Timeout:          10 * time.Second,
//	        HalfOpenRequests: 2,
//	    },
//	}
//	ferry, err := charon.NewBoatFerry(cfg)
//
//	ferry.RegisterShore(&charon.Shore{
//	    ID:      "olympus-1",
//	    Address: "http://olympus-1:8080",
//	    Weight:  10,
//	    Zone:    "us-east-1a",
//	})
//
//	// Wrap your HTTP mux with Charon middleware to enable streaming support:
//	mw := charon.NewFerryMiddleware(ferry)
//	http.Handle("/", mw.Handler(yourMux))
//
// # mTLS Health Probes
//
//	hc, err := charon.NewMTLSHealthCheck("client.crt", "client.key", "ca.crt", "/health")
//	ferry.RegisterShore(&charon.Shore{
//	    ID:          "secure-backend",
//	    Address:     "https://backend:8443",
//	    HealthCheck: hc,
//	})
package charon
