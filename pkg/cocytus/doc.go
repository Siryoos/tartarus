// Package cocytus implements the error stream — the River of Wailing that
// collects, categorises, and routes system-wide failures.
//
// Named after the river of wailing in Hades, Cocytus acts as the error
// pipeline for the Tartarus platform. It exposes a [Sink] interface that
// receives failed [domain.SandboxRun] records and routes them to one or
// more downstream handlers (logging, alerting, dead-letter storage).
//
// # Implementations
//
//   - [LogSink]: Writes JSON-encoded failure records to a structured logger.
//     Suitable for development and low-volume deployments.
//
// # Basic Usage
//
//	sink := cocytus.NewLogSink(logger)
//	// On failure:
//	sink.Record(ctx, &domain.SandboxRun{...}, "timeout exceeded")
//
// # Known Technical Debt
//
//   - Only a [LogSink] is provided. Production deployments typically require
//     a durable sink (e.g., persisting to the Acheron DLQ stream or sending
//     to a Slack/PagerDuty webhook). These are not yet implemented.
//
//   - There is no fan-out (multi-sink) implementation. The interface accepts
//     a single Sink, but many systems need simultaneous logging + alerting.
//     A MultiSink wrapper should be added.
//
//   - Failed-record replay is entirely external. Cocytus only records
//     failures; it does not provide any retry or replay capability.
package cocytus
