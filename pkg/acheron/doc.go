// Package acheron implements the durable job queue — the River of Pain that
// every sandbox request must cross before reaching the execution layer.
//
// Named after the river that souls must cross to enter the Underworld, Acheron
// provides at-least-once delivery semantics with a dead-letter queue (DLQ) for
// failed or poison-pill messages.
//
// # Implementations
//
// Three Queue implementations are provided:
//
//   - [RedisQueue]: Production implementation backed by a Redis Stream.
//     Supports consumer groups, automatic re-delivery on timeout (PEL),
//     configurable retry limits, and DLQ promotion via atomic Lua scripts.
//
//   - [RedisClusterQueue]: Redis Cluster-aware variant of RedisQueue.
//     Keys are automatically hash-tagged so the stream and its DLQ always
//     reside on the same shard, satisfying the cross-key Lua script constraint.
//
//   - [MemoryQueue]: In-process implementation for unit tests and
//     single-node development deployments. Does not persist across restarts.
//     Dequeue fully respects context cancellation via a channel-based notify.
//
//   - [PriorityQueue]: Multi-tier fan-in queue backed by one [MemoryQueue]
//     per [domain.Priority] level. Higher-priority items are always dequeued
//     first. Dequeue respects context cancellation.
//
// # Basic Usage
//
//	q := acheron.NewRedisQueue(redisAddr, 0, "tartarus:jobs", "workers", "node-1", false, metrics, nil)
//
//	// Producer side
//	q.Enqueue(ctx, &domain.SandboxRequest{Priority: domain.PriorityHigh, ...})
//
//	// Consumer side
//	req, receipt, err := q.Dequeue(ctx)
//	if err != nil { ... }
//	if err := process(req); err != nil {
//	    q.Nack(ctx, receipt, err.Error()) // re-enqueue or move to DLQ
//	} else {
//	    q.Ack(ctx, receipt)
//	}
//
// # Dead-Letter Queue
//
// When a message exceeds [RedisQueue]'s MaxRetries, it is atomically moved to
// a separate DLQ stream ("<stream>:dlq") with the original payload and an error
// annotation. Operators can inspect and replay DLQ entries out-of-band.
//
// # Telemetry
//
// All implementations emit the same Prometheus-compatible metric names via a
// [hermes.Metrics] sink:
//
//   - queue_enqueue_total / queue_enqueue_errors_total
//   - queue_dequeue_total
//   - queue_nack_total / queue_nack_errors_total
//   - queue_depth (gauge)
//   - queue_dlq_depth (gauge, RedisQueue / RedisClusterQueue only)
//
// Use [NewInstrumentedQueue] to wrap any [Queue] with the shared metrics layer.
//
// # Priority Scheduling
//
// Use [domain.Priority] constants (PriorityLow, PriorityNormal, PriorityHigh)
// to annotate [domain.SandboxRequest] before enqueueing. [PriorityQueue]
// routes each request to the appropriate tier and always dequeues the highest
// non-empty tier first.
package acheron
