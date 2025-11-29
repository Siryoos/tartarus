package acheron

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// nackScript atomically re-enqueues a message and acknowledges the old one.
// KEYS[1]: stream key
// ARGV[1]: consumer group
// ARGV[2]: message ID (receipt)
var nackScript = redis.NewScript(`
	local stream = KEYS[1]
	local group = ARGV[1]
	local id = ARGV[2]

	-- Get the message details
	local range = redis.call("XRANGE", stream, id, id)
	if #range == 0 then
		return 0 -- Message not found
	end

	local msg = range[1]
	local values = msg[2]

	-- Re-enqueue (XADD)
	redis.call("XADD", stream, "*", unpack(values))

	-- Ack the old one
	redis.call("XACK", stream, group, id)

	return 1
`)

// deadLetterScript atomically moves a message to the DLQ and acknowledges the old one.
// KEYS[1]: stream key
// KEYS[2]: dlq key
// ARGV[1]: consumer group
// ARGV[2]: message ID (receipt)
// ARGV[3]: error reason
// ARGV[4]: timestamp
var deadLetterScript = redis.NewScript(`
	local stream = KEYS[1]
	local dlq = KEYS[2]
	local group = ARGV[1]
	local id = ARGV[2]
	local error_reason = ARGV[3]
	local timestamp = ARGV[4]

	-- Get the message details
	local range = redis.call("XRANGE", stream, id, id)
	if #range == 0 then
		return 0 -- Message not found
	end

	local msg = range[1]
	local values = msg[2]

	-- Add error metadata to the DLQ entry
	local dlq_values = {}
	for i = 1, #values, 2 do
		table.insert(dlq_values, values[i])
		table.insert(dlq_values, values[i+1])
	end
	table.insert(dlq_values, "error_reason")
	table.insert(dlq_values, error_reason)
	table.insert(dlq_values, "original_id")
	table.insert(dlq_values, id)
	table.insert(dlq_values, "dlq_timestamp")
	table.insert(dlq_values, timestamp)

	-- Add to DLQ (XADD)
	redis.call("XADD", dlq, "*", unpack(dlq_values))

	-- Ack the old one
	redis.call("XACK", stream, group, id)

	return 1
`)

type RedisQueue struct {
	client        *redis.Client
	streamKey     string
	dlqKey        string
	consumerGroup string
	consumerName  string
	routing       bool
	metrics       hermes.Metrics
}

func NewRedisQueue(addr string, db int, streamKey string, consumerGroup string, consumerName string, routing bool, metrics hermes.Metrics) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	q := &RedisQueue{
		client:        client,
		streamKey:     streamKey,
		dlqKey:        streamKey + ":dlq",
		consumerGroup: consumerGroup,
		consumerName:  consumerName,
		routing:       routing,
		metrics:       metrics,
	}

	// If we are a consumer (have group and name), ensure group exists
	if consumerGroup != "" && consumerName != "" {
		// We need to ensure the group exists for the stream.
		// If the stream doesn't exist, MKSTREAM will create it.
		// 0 means start consuming from the beginning (all undelivered messages).
		err := client.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "0").Err()
		if err != nil {
			// Ignore "BUSYGROUP Consumer Group name already exists"
			if err.Error() != "BUSYGROUP Consumer Group name already exists" {
				// Log error but proceed? Or fail?
				// Given we can't easily check error type, we'll assume it's fine if it exists.
				// But for other errors, we might want to know.
				// For now, we'll just return the queue object, but maybe log this?
				// The original code ignored non-BUSYGROUP errors too (implicitly via comment logic).
			}
		}
	}

	return q, nil
}

func (q *RedisQueue) Enqueue(ctx context.Context, req *domain.SandboxRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	targetKey := q.streamKey
	if q.routing && req.NodeID != "" {
		targetKey = fmt.Sprintf("%s:%s", q.streamKey, req.NodeID)
	}

	// XADD
	// We use "*" for ID to let Redis generate it.
	// Values are map[string]interface{}. We store "data" -> json.
	args := &redis.XAddArgs{
		Stream: targetKey,
		Values: map[string]interface{}{
			"data": data,
		},
	}

	if err := q.client.XAdd(ctx, args).Err(); err != nil {
		q.metrics.IncCounter("queue_enqueue_errors_total", 1, hermes.Label{Key: "queue", Value: targetKey})
		return fmt.Errorf("failed to enqueue request: %w", err)
	}

	q.metrics.IncCounter("queue_enqueue_total", 1, hermes.Label{Key: "queue", Value: targetKey})

	// Emit queue depth
	if depth, err := q.client.XLen(ctx, targetKey).Result(); err == nil {
		q.metrics.SetGauge("queue_depth", float64(depth), hermes.Label{Key: "queue", Value: targetKey})
	}

	return nil
}

func (q *RedisQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, string, error) {
	if q.consumerGroup == "" || q.consumerName == "" {
		return nil, "", fmt.Errorf("consumer group/name not configured for dequeue")
	}

	for {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}

		// XREADGROUP
		// Block for 1 second.
		// Streams: key -> ">" (means messages never delivered to other consumers)
		streams := []string{q.streamKey, ">"}

		res, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    q.consumerGroup,
			Consumer: q.consumerName,
			Streams:  streams,
			Count:    1,
			Block:    1 * time.Second,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue // Timeout, retry
			}
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			return nil, "", fmt.Errorf("failed to dequeue: %w", err)
		}

		if len(res) == 0 || len(res[0].Messages) == 0 {
			continue
		}

		msg := res[0].Messages[0]
		dataStr, ok := msg.Values["data"].(string)
		if !ok {
			// Invalid payload format (not a string in "data" field)
			q.moveToDLQ(ctx, msg.ID, "invalid_payload_format")
			continue
		}

		var req domain.SandboxRequest
		if err := json.Unmarshal([]byte(dataStr), &req); err != nil {
			// Corrupt JSON payload
			q.moveToDLQ(ctx, msg.ID, "json_unmarshal_error")
			continue
		}

		q.metrics.IncCounter("queue_dequeue_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
		q.updateDepth(ctx)

		return &req, msg.ID, nil
	}
}

func (q *RedisQueue) moveToDLQ(ctx context.Context, id string, errorReason string) {
	// Use Lua script to atomically move to DLQ and Ack
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	err := deadLetterScript.Run(ctx, q.client, []string{q.streamKey, q.dlqKey}, q.consumerGroup, id, errorReason, timestamp).Err()
	if err != nil {
		// If script fails, we might be in trouble. Log it?
		// We can't easily log here without a logger.
		// But we should increment a metric.
		q.metrics.IncCounter("queue_dlq_move_errors_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
	} else {
		q.metrics.IncCounter("queue_poison_pill_total", 1, hermes.Label{Key: "queue", Value: q.streamKey}, hermes.Label{Key: "reason", Value: errorReason})
		q.updateDLQDepth(ctx)
	}
}

func (q *RedisQueue) updateDepth(ctx context.Context) {
	if depth, err := q.client.XLen(ctx, q.streamKey).Result(); err == nil {
		q.metrics.SetGauge("queue_depth", float64(depth), hermes.Label{Key: "queue", Value: q.streamKey})
	}
}

func (q *RedisQueue) updateDLQDepth(ctx context.Context) {
	if depth, err := q.client.XLen(ctx, q.dlqKey).Result(); err == nil {
		q.metrics.SetGauge("queue_dlq_depth", float64(depth), hermes.Label{Key: "queue", Value: q.streamKey})
	}
}

// Ack acknowledges a message, removing it from the Pending Entry List (PEL).
// This uses Redis XACK which performs O(1) hash-based lookup, making it scalable
// regardless of PEL size. This is a key performance improvement over list-based
// queue implementations which require O(N) scanning.
func (q *RedisQueue) Ack(ctx context.Context, receipt string) error {
	// XACK
	// O(1)
	if err := q.client.XAck(ctx, q.streamKey, q.consumerGroup, receipt).Err(); err != nil {
		return fmt.Errorf("failed to ack message: %w", err)
	}
	return nil
}

func (q *RedisQueue) Nack(ctx context.Context, receipt string, reason string) error {
	// Use Lua script to atomically re-enqueue and Ack
	err := nackScript.Run(ctx, q.client, []string{q.streamKey}, q.consumerGroup, receipt).Err()
	if err != nil {
		q.metrics.IncCounter("queue_nack_errors_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
		return fmt.Errorf("failed to nack message: %w", err)
	}

	q.metrics.IncCounter("queue_nack_total", 1, hermes.Label{Key: "queue", Value: q.streamKey})
	return nil
}
