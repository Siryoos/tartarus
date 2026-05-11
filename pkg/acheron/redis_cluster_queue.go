package acheron

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// RedisClusterQueue is a Redis Streams-backed Queue that targets a Redis
// Cluster deployment (≥ 3 primary shards).  It is API-compatible with
// RedisQueue and reuses the same Lua scripts for atomic Nack / DLQ promotion.
//
// # Hash-slot constraint
//
// Redis Cluster requires all keys used within a single command or Lua script
// to reside on the same hash slot.  RedisClusterQueue enforces this by
// wrapping the user-supplied stream key in curly braces so that Redis uses
// only the bracketed portion to compute the slot:
//
//	"tartarus:jobs"     → stored as "{tartarus:jobs}"
//	"tartarus:jobs:dlq" → stored as "{tartarus:jobs}:dlq"
//
// Both keys hash to the same slot, satisfying the Cluster constraint for the
// two-key Lua scripts (nackScript, deadLetterScript).
//
// # No automated tests
//
// Integration-testing a Cluster requires ≥ 3 real nodes.  Tests are skipped
// by default; see the build-tag note in the package doc.
type RedisClusterQueue struct {
	client        *redis.ClusterClient
	streamKey     string // already hash-tagged, e.g. "{tartarus:jobs}"
	dlqKey        string // "{tartarus:jobs}:dlq"
	consumerGroup string
	consumerName  string
	metrics       hermes.Metrics
	sink          cocytus.Sink
}

// RedisClusterQueueConfig holds the constructor parameters for RedisClusterQueue.
type RedisClusterQueueConfig struct {
	// Addrs is the seed list of cluster node addresses (host:port).
	// At least one reachable node is required; the client discovers the rest.
	Addrs []string

	// StreamKey is the logical queue name.  It will be wrapped in hash tags
	// automatically; do NOT pre-tag it.
	StreamKey string

	// ConsumerGroup and ConsumerName identify this consumer within the group.
	// Leave both empty to create a producer-only instance.
	ConsumerGroup string
	ConsumerName  string

	// Metrics sink for Prometheus-compatible instrumentation.
	Metrics hermes.Metrics

	// Sink is an optional Cocytus error stream for poison-pill audit trail.
	Sink cocytus.Sink
}

// NewRedisClusterQueue connects to a Redis Cluster and returns a ready-to-use
// RedisClusterQueue.  The consumer group is created (MKSTREAM) if it does not
// yet exist.
func NewRedisClusterQueue(cfg RedisClusterQueueConfig) (*RedisClusterQueue, error) {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: cfg.Addrs,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("acheron: failed to connect to Redis Cluster: %w", err)
	}

	// Wrap the stream key in a hash tag so both the stream and its DLQ land
	// on the same cluster shard, satisfying the Lua script constraint.
	taggedKey := "{" + cfg.StreamKey + "}"
	dlqKey := taggedKey + ":dlq"

	q := &RedisClusterQueue{
		client:        client,
		streamKey:     taggedKey,
		dlqKey:        dlqKey,
		consumerGroup: cfg.ConsumerGroup,
		consumerName:  cfg.ConsumerName,
		metrics:       cfg.Metrics,
		sink:          cfg.Sink,
	}

	if cfg.ConsumerGroup != "" && cfg.ConsumerName != "" {
		err := client.XGroupCreateMkStream(ctx, taggedKey, cfg.ConsumerGroup, "0").Err()
		if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
			// Non-fatal: log via metric and proceed; the group may already exist.
			q.metrics.IncCounter("queue_group_create_errors_total", 1,
				hermes.Label{Key: "queue", Value: taggedKey})
		}
	}

	return q, nil
}

// Enqueue serialises req and appends it to the cluster stream using XADD.
func (q *RedisClusterQueue) Enqueue(ctx context.Context, req *domain.SandboxRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("acheron: failed to marshal request: %w", err)
	}

	args := &redis.XAddArgs{
		Stream: q.streamKey,
		Values: map[string]interface{}{"data": data},
	}

	if err := q.client.XAdd(ctx, args).Err(); err != nil {
		q.metrics.IncCounter("queue_enqueue_errors_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
		return fmt.Errorf("acheron: failed to enqueue: %w", err)
	}

	q.metrics.IncCounter("queue_enqueue_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
	if depth, err := q.client.XLen(ctx, q.streamKey).Result(); err == nil {
		q.metrics.SetGauge("queue_depth", float64(depth), hermes.Label{Key: "queue", Value: q.streamKey})
	}
	return nil
}

// Dequeue blocks until a message is available or ctx is cancelled.
// Corrupt / unparseable messages are moved to the DLQ automatically.
func (q *RedisClusterQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, string, error) {
	if q.consumerGroup == "" || q.consumerName == "" {
		return nil, "", fmt.Errorf("acheron: consumer group/name not configured for dequeue")
	}

	for {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}

		res, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    q.consumerGroup,
			Consumer: q.consumerName,
			Streams:  []string{q.streamKey, ">"},
			Count:    1,
			Block:    time.Second,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
			}
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			return nil, "", fmt.Errorf("acheron: dequeue failed: %w", err)
		}

		if len(res) == 0 || len(res[0].Messages) == 0 {
			continue
		}

		msg := res[0].Messages[0]
		dataStr, ok := msg.Values["data"].(string)
		if !ok {
			q.clusterMoveToDLQ(ctx, msg.ID, "invalid_payload_format")
			continue
		}

		var req domain.SandboxRequest
		if err := json.Unmarshal([]byte(dataStr), &req); err != nil {
			q.clusterMoveToDLQ(ctx, msg.ID, "json_unmarshal_error")
			continue
		}

		q.metrics.IncCounter("queue_dequeue_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
		if depth, err := q.client.XLen(ctx, q.streamKey).Result(); err == nil {
			q.metrics.SetGauge("queue_depth", float64(depth), hermes.Label{Key: "queue", Value: q.streamKey})
		}
		return &req, msg.ID, nil
	}
}

// Ack acknowledges a message via XACK, removing it from the PEL. O(1).
func (q *RedisClusterQueue) Ack(ctx context.Context, receipt string) error {
	if err := q.client.XAck(ctx, q.streamKey, q.consumerGroup, receipt).Err(); err != nil {
		return fmt.Errorf("acheron: ack failed: %w", err)
	}
	return nil
}

// Nack atomically re-enqueues the message and acknowledges the original entry
// using the shared nackScript Lua script.
func (q *RedisClusterQueue) Nack(ctx context.Context, receipt string, reason string) error {
	err := nackScript.Run(ctx, q.client, []string{q.streamKey}, q.consumerGroup, receipt).Err()
	if err != nil {
		q.metrics.IncCounter("queue_nack_errors_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
		return fmt.Errorf("acheron: nack failed: %w", err)
	}
	q.metrics.IncCounter("queue_nack_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
	return nil
}

// Len returns the current depth of the stream via XLEN.
func (q *RedisClusterQueue) Len(ctx context.Context) int {
	depth, err := q.client.XLen(ctx, q.streamKey).Result()
	if err != nil {
		return 0
	}
	return int(depth)
}

// clusterMoveToDLQ atomically moves a poison-pill message to the DLQ using
// the shared deadLetterScript.  Because both stream and DLQ keys share the
// same hash tag they always reside on the same shard.
func (q *RedisClusterQueue) clusterMoveToDLQ(ctx context.Context, id string, errorReason string) {
	if q.sink != nil {
		rangeRes, err := q.client.XRange(ctx, q.streamKey, id, id).Result()
		if err == nil && len(rangeRes) > 0 {
			msg := rangeRes[0]
			var payload []byte
			if dataStr, ok := msg.Values["data"].(string); ok {
				payload = []byte(dataStr)
			}
			rec := &cocytus.Record{
				RequestID: domain.SandboxID(id),
				Reason:    fmt.Sprintf("poison_pill: %s", errorReason),
				Payload:   payload,
				CreatedAt: time.Now(),
			}
			cocytusCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if wErr := q.sink.Write(cocytusCtx, rec); wErr != nil {
				q.metrics.IncCounter("queue_poison_pill_cocytus_write_errors_total", 1,
					hermes.Label{Key: "queue", Value: q.streamKey})
			}
		}
	}

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	err := deadLetterScript.Run(ctx, q.client,
		[]string{q.streamKey, q.dlqKey},
		q.consumerGroup, id, errorReason, timestamp,
	).Err()
	if err != nil {
		q.metrics.IncCounter("queue_dlq_move_errors_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
	} else {
		q.metrics.IncCounter("queue_poison_pill_total", 1,
			hermes.Label{Key: "queue", Value: q.streamKey},
			hermes.Label{Key: "reason", Value: errorReason})
		if depth, err := q.client.XLen(ctx, q.dlqKey).Result(); err == nil {
			q.metrics.SetGauge("queue_dlq_depth", float64(depth), hermes.Label{Key: "queue", Value: q.streamKey})
		}
	}
}
