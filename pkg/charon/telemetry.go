package charon

import (
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// Telemetry tracks and exports metrics for the Charon ferry.
type Telemetry struct {
	metrics hermes.Metrics
}

// NewTelemetry creates a new telemetry exporter.
func NewTelemetry(metrics hermes.Metrics) *Telemetry {
	return &Telemetry{
		metrics: metrics,
	}
}

// RecordRequest records a request to a backend shore.
func (t *Telemetry) RecordRequest(shoreID string, success bool, duration time.Duration) {
	if t.metrics == nil {
		return
	}

	status := "success"
	if !success {
		status = "failure"
	}

	// Counter: Total requests
	t.metrics.IncCounter("charon_requests_total", 1,
		hermes.Label{Key: "shore_id", Value: shoreID},
		hermes.Label{Key: "status", Value: status},
	)

	// Histogram: Request duration
	t.metrics.ObserveHistogram("charon_request_duration_seconds", duration.Seconds(),
		hermes.Label{Key: "shore_id", Value: shoreID},
	)
}

// RecordCircuitBreakerState records the state of a circuit breaker.
func (t *Telemetry) RecordCircuitBreakerState(shoreID string, state CircuitBreakerState) {
	if t.metrics == nil {
		return
	}

	// Gauge: 1 for current state, 0 for others
	for _, s := range []CircuitBreakerState{StateClosed, StateOpen, StateHalfOpen} {
		value := 0.0
		if s == state {
			value = 1.0
		}
		t.metrics.SetGauge("charon_circuit_breaker_state", value,
			hermes.Label{Key: "shore_id", Value: shoreID},
			hermes.Label{Key: "state", Value: s.String()},
		)
	}
}

// RecordActiveConnections records the current number of active connections.
func (t *Telemetry) RecordActiveConnections(shoreID string, count int) {
	if t.metrics == nil {
		return
	}

	t.metrics.SetGauge("charon_active_connections", float64(count),
		hermes.Label{Key: "shore_id", Value: shoreID},
	)
}

// RecordHealthCheck records the result of a health check.
func (t *Telemetry) RecordHealthCheck(shoreID string, success bool, latency time.Duration) {
	if t.metrics == nil {
		return
	}

	result := "success"
	if !success {
		result = "failure"
	}

	// Counter: Health check results
	t.metrics.IncCounter("charon_health_check_total", 1,
		hermes.Label{Key: "shore_id", Value: shoreID},
		hermes.Label{Key: "result", Value: result},
	)

	// Histogram: Health check latency (only for successful checks)
	if success {
		t.metrics.ObserveHistogram("charon_health_check_duration_seconds", latency.Seconds(),
			hermes.Label{Key: "shore_id", Value: shoreID},
		)
	}
}

// RecordShoreHealth records the health status of a shore.
func (t *Telemetry) RecordShoreHealth(shoreID string, status HealthStatus) {
	if t.metrics == nil {
		return
	}

	// Gauge: 1 for current status, 0 for others
	for _, s := range []HealthStatus{HealthStatusHealthy, HealthStatusDegraded, HealthStatusUnhealthy} {
		value := 0.0
		if s == status {
			value = 1.0
		}
		t.metrics.SetGauge("charon_shore_health", value,
			hermes.Label{Key: "shore_id", Value: shoreID},
			hermes.Label{Key: "status", Value: string(s)},
		)
	}
}

// RecordRateLimitHit records when a request is rate limited.
func (t *Telemetry) RecordRateLimitHit(key string) {
	if t.metrics == nil {
		return
	}

	t.metrics.IncCounter("charon_rate_limit_hits_total", 1,
		hermes.Label{Key: "key", Value: key},
	)
}

// NoOpTelemetry is a telemetry implementation that does nothing.
type NoOpTelemetry struct{}

// NewNoOpTelemetry creates a no-op telemetry.
func NewNoOpTelemetry() *NoOpTelemetry {
	return &NoOpTelemetry{}
}

func (t *NoOpTelemetry) RecordRequest(shoreID string, success bool, duration time.Duration)  {}
func (t *NoOpTelemetry) RecordCircuitBreakerState(shoreID string, state CircuitBreakerState) {}
func (t *NoOpTelemetry) RecordActiveConnections(shoreID string, count int)                   {}
func (t *NoOpTelemetry) RecordHealthCheck(shoreID string, success bool, latency time.Duration) {
}
func (t *NoOpTelemetry) RecordShoreHealth(shoreID string, status HealthStatus) {}
func (t *NoOpTelemetry) RecordRateLimitHit(key string)                         {}
