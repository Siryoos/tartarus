// Package hermes implements telemetry and observability — the Messenger God
// who carries metrics, logs, and traces across the system.
//
// Named after the swift messenger of the gods, Hermes provides a unified
// observability façade. All other packages in this repository depend on
// hermes interfaces rather than specific observability libraries, allowing
// the telemetry backend to be swapped without touching business logic.
//
// # Sub-packages
//
//   - [hermes/audit]: Tamper-evident audit log with HMAC-SHA256 event chaining.
//     Supports LogStore, FileStore, and TamperEvidentStore.
//
// # Core Interfaces
//
//   - [Metrics]: Counter and histogram façade (implemented by
//     [PrometheusMetrics] and a no-op for testing).
//
//   - [Logger]: Structured levelled log interface backed by log/slog.
//
// # Prometheus Integration
//
// [PrometheusMetrics] registers and exposes metrics on the standard
// prometheus.DefaultRegisterer. Metric names follow the pattern:
//
//	<namespace>_<name>_total   (counters)
//	<namespace>_<name>_seconds (histograms)
//
// # Basic Usage
//
//	met := hermes.NewPrometheusMetrics("tartarus")
//	log := hermes.NewSlogLogger(slog.Default())
//
//	met.IncCounter("sandbox_submissions_total", 1)
//	met.ObserveHistogram("submission_duration_seconds", latency.Seconds())
//	log.Info(ctx, "Sandbox submitted", map[string]any{"id": id})
//
// # Known Technical Debt
//
//   - Distributed tracing (OpenTelemetry/Jaeger) is listed in the package
//     description but the [observability.go] file contains only stubs.
//     No spans are propagated across RPC or Pub/Sub calls.
//
//   - The [Metrics] interface only supports counters and histograms. Gauges
//     (e.g., for live sandbox count, queue depth) require direct use of the
//     Prometheus SDK, bypassing the façade and complicating testing.
//
//   - [hermes/audit] [Store] interface only has a Write method; the Read
//     method is commented out as a placeholder. Audit retrieval/search
//     (e.g., for compliance queries) is completely unimplemented.
//
//   - The [StandardAuditor] runs anomaly detectors synchronously in the hot
//     path of every request. Slow or blocking detectors will increase
//     latency for all callers. An async queue for anomaly processing is
//     recommended.
package hermes
