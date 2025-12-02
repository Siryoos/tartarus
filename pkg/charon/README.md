# Charon Load Balancer

Charon is the production load balancer for Tartarus Olympus instances. Named after the ferryman who transports souls across the River Styx, Charon ensures that requests are reliably routed to healthy Olympus backends.

## Features

### ✅ Health Checks
- Configurable HTTP health check endpoints
- Automatic detection of failed backends
- Gradual health recovery with thresholds

### ✅ Load Balancing Strategies
- **Round Robin**: Simple round-robin distribution
- **Least Connections**: Route to backend with fewest active connections
- **Weighted Random**: Weight-based random selection
- **IP Hash**: Simple IP-based hashing (legacy)
- **Consistent Hash**: Advanced consistent hashing with virtual nodes for sticky sessions
- **Zone Aware**: Prefer backends in the same availability zone (planned)

### ✅ Circuit Breakers
- Protect backends from cascading failures
- Three states: Closed (normal), Open (rejecting), Half-Open (testing)
- Automatic recovery with configurable thresholds and timeouts

### ✅ Sticky Sessions
- Session affinity using consistent hashing
- Supports multiple affinity keys: IP, tenant ID, session ID, or custom header
- Minimal disruption when backends are added/removed

### ✅ Rate Limiting
- Token bucket algorithm with burst support
- Tenant-aware, IP-based, or identity-based limiting
- Redis-backed distributed rate limiting (optional)

### ✅ Telemetry
- Prometheus metrics export
- Request counts, latencies, and success rates per backend
- Circuit breaker state monitoring
- Health check results

## Quick Start

### Using Environment Variables

```bash
# Set Olympus backend addresses
export CHARON_SHORE_OLYMPUS1=http://olympus-1:8080
export CHARON_SHORE_OLYMPUS2=http://olympus-2:8080
export CHARON_SHORE_OLYMPUS3=http://olympus-3:8080

# Optional: Redis for distributed rate limiting
export REDIS_ADDR=redis:6379

# Start Charon
./charon-proxy --listen=:8000
```

### Using Configuration File

```bash
./charon-proxy --config=config/charon.json
```

See [config/charon.json](../config/charon.json) for a complete example.

## Configuration

### Load Balancing Strategy

Choose a strategy based on your use case:

```json
{
  "ferry": {
    "strategy": "consistent_hash",
    "session_affinity_key": "tenant"
  }
}
```

**Strategy Options:**
- `round_robin`: Best for stateless workloads with uniform requests
- `least_conn`: Best for workloads with varying request durations
- `weighted`: Best when backends have different capacities
- `consistent_hash`: **Best for sticky sessions** (recommended for production)

**Session Affinity Keys:**
- `ip`: Route based on client IP address
- `tenant`: Route based on `X-Tenant-ID` header
- `session`: Route based on `session_id` cookie or `X-Session-ID` header
- `custom`: Route based on `X-Affinity-Key` header

### Circuit Breaker

Protect backends from overload:

```json
{
  "circuit_breaker": {
    "enabled": true,
    "threshold": 5,
    "timeout": "30s",
    "half_open_requests": 3
  }
}
```

- `threshold`: Number of consecutive failures before opening the circuit
- `timeout`: How long to wait before testing recovery
- `half_open_requests`: Number of test requests in half-open state

### Health Checks

Configure health monitoring per backend:

```json
{
  "shores": [
    {
      "id": "olympus-1",
      "address": "http://olympus-1:8080",
      "health_check": {
        "path": "/health",
        "interval": "10s",
        "timeout": "5s",
        "healthy": 2,
        "unhealthy": 3
      }
    }
  ]
}
```

- `path`: HTTP endpoint to check
- `interval`: Time between health checks
- `timeout`: Request timeout
- `healthy`: Consecutive successes to mark healthy
- `unhealthy`: Consecutive failures to mark unhealthy

### Rate Limiting

Prevent abuse and ensure fair usage:

```json
{
  "rate_limiting": {
    "enabled": true,
    "requests_per_second": 1000,
    "burst": 2000,
    "key_func": "tenant",
    "redis_addr": "redis:6379"
  }
}
```

- `requests_per_second`: Sustained rate limit
- `burst`: Maximum burst capacity
- `key_func`: How to identify clients (`tenant`, `ip`, `identity`)
- `redis_addr`: Optional Redis for distributed limiting

## Metrics

Charon exports Prometheus metrics at `/metrics`:

```
# Request metrics
charon_requests_total{shore_id, status}
charon_request_duration_seconds{shore_id}

# Connection metrics
charon_active_connections{shore_id}

# Health metrics
charon_health_check_total{shore_id, result}
charon_health_check_duration_seconds{shore_id}
charon_shore_health{shore_id, status}

# Circuit breaker metrics
charon_circuit_breaker_state{shore_id, state}

# Rate limiting metrics
charon_rate_limit_hits_total{key}
```

### Grafana Dashboard

Import the pre-built Charon dashboard (coming soon) to visualize:
- Request rate and latency per backend
- Backend health status
- Circuit breaker states
- Active connections

## High Availability

For production deployments:

1. **Multiple Charon Instances**: Run Charon behind an external load balancer (e.g., AWS ALB/NLB)
2. **Redis-backed Rate Limiting**: Share rate limit state across Charon instances
3. **Service Discovery**: Use Hades integration for dynamic backend discovery (planned)
4. **Health Monitoring**: Alert on degraded/unhealthy status via Prometheus

Example HA configuration: [config/charon-ha.json](../config/charon-ha.json)

## Best Practices

1. **Use Consistent Hashing**: For stable session routing in production
2. **Tune Circuit Breakers**: Balance between fast failure detection and tolerance for transient errors
3. **Monitor Metrics**: Set up alerts for high error rates and circuit breaker openings
4. **Health Check Intervals**: 10-30s is usually sufficient; avoid too frequent checks
5. **Rate Limiting**: Use tenant-based limiting to isolate noisy neighbors

## Troubleshooting

### All Requests Failing

Check ferry health:
```bash
curl http://localhost:8000/health
```

If status is `unhealthy`, all backends may be down. Check:
1. Backend connectivity
2. Health check configuration
3. Circuit breaker states (may need manual reset)

### Sticky Sessions Not Working

Verify:
1. Strategy is set to `consistent_hash`
2. Correct `session_affinity_key` is configured
3. Clients are sending the affinity header/cookie

### High Latency

Check:
- Backend response times via metrics
- Circuit breaker timeouts (may be too long)
- Network connectivity between Charon and backends

## Development

### Running Tests

```bash
# Unit tests
go test -v ./pkg/charon/...

# Integration tests
go test -v ./tests/integration -run TestCharon
```

### Benchmarks

```bash
go test -bench=. ./pkg/charon/...
```

## See Also

- [Architecture Documentation](../docs/architecture.md)
- [Mythology Guide](../docs/mythology.md)
- [Olympus API Documentation](../docs/api/olympus.md)
