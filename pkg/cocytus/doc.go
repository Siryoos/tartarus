// Package cocytus implements the error stream — the River of Wailing that
// collects, categorises, and routes system-wide failures.
//
// Named after the river of wailing in Hades, Cocytus acts as the error
// pipeline for the Tartarus platform. It exposes a [Sink] interface that
// receives failed [Record] objects and routes them to one or more downstream
// handlers (logging, alerting, dead-letter storage).
//
// # Implementations
//
//   - [LogSink]: Writes structured failure records to a [log/slog] logger.
//     Suitable for development and low-volume deployments.
//
//   - [AcheronSink]: Durable sink that persists records to a Redis Stream
//     (the Acheron DLQ). Survives process restarts; suitable for production.
//
//   - [WebhookSink]: Alert sink that HTTP-POSTs records as JSON to any
//     webhook URL. Compatible with Slack incoming webhooks and the
//     PagerDuty Events v2 API.
//
//   - [MultiSink]: Fan-out wrapper that delivers each record to every
//     registered sink simultaneously. Errors are joined via [errors.Join].
//
//   - [ReplaySink]: Retry decorator that wraps any Sink with bounded
//     retry logic and an optional dead-letter fallback Sink.
//
// # Basic Usage
//
//	logger := slog.Default()
//	logSink := cocytus.NewLogSink(logger)
//
//	dlq, _ := cocytus.NewAcheronSink(cocytus.AcheronSinkConfig{
//	    Addr:      "redis:6379",
//	    StreamKey: "cocytus:dlq",
//	})
//
//	webhook, _ := cocytus.NewWebhookSink(cocytus.WebhookSinkConfig{
//	    URL: "https://hooks.slack.com/services/...",
//	})
//
//	multi, _ := cocytus.NewMultiSink(logSink, dlq, webhook)
//
//	sink, _ := cocytus.NewReplaySink(cocytus.ReplayConfig{
//	    Inner:       multi,
//	    MaxAttempts: 3,
//	    Backoff:     500 * time.Millisecond,
//	    Fallback:    logSink,
//	})
//
//	// On failure:
//	sink.Write(ctx, &cocytus.Record{
//	    RunID:     runID,
//	    Reason:    "timeout exceeded",
//	    CreatedAt: time.Now(),
//	})
package cocytus
