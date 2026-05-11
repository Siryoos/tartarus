// Package acheron implements the durable job queue — the River of Pain that
// every sandbox request must cross before reaching the execution layer.
//
// Named after the river that souls must cross to enter the Underworld, Acheron
// provides at-least-once delivery semantics with a dead-letter queue (DLQ) for
// failed or poison-pill messages.
//
// # Implementations
//
// Two Queue implementations are provided:
//
//   - [RedisQueue]: Production implementation backed by a Redis Stream.
//     Supports consumer groups, automatic re-delivery on timeout (PEL),
//     configurable retry limits, and DLQ promotion via atomic Lua scripts.
//
//   - [MemoryQueue]: In-process implementation for unit tests and
//     single-node development deployments. Does not persist across restarts.
//
// # Basic Usage
//
//	q := acheron.NewRedisQueue(redisClient, acheron.RedisQueueConfig{
//	    StreamKey:    "tartarus:jobs",
//	    Group:        "workers",
//	    Consumer:     "node-1",
//	    MaxRetries:   5,
//	    VisibilityTimeout: 30 * time.Second,
//	})
//
//	// Producer side
//	q.Enqueue(ctx, &domain.SandboxRequest{...})
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
// # Known Technical Debt
//
//   - [MemoryQueue.Dequeue] uses sync.Cond.Wait which cannot be interrupted by
//     context cancellation mid-wait. A channel-based or polling approach is
//     needed for correct graceful shutdown in long-idle scenarios.
//
//   - Prometheus metrics are emitted only by [RedisQueue]. MemoryQueue has no
//     telemetry; consider extracting a shared metrics wrapper.
//
//   - Priority queuing is not implemented. All messages are FIFO within a
//     single stream. A multi-stream fan-in approach (e.g., separate streams
//     per priority class) would be needed to support priority scheduling.
//
//   - RedisQueue does not support cross-shard distribution. For very large
//     deployments a Redis Cluster-aware sharding layer would be required.
package acheron
