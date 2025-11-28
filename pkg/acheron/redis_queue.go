package acheron

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type RedisQueue struct {
	client  *redis.Client
	key     string
	routing bool
}

func NewRedisQueue(addr string, db int, key string, routing bool) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisQueue{
		client:  client,
		key:     key,
		routing: routing,
	}, nil
}

func (q *RedisQueue) Enqueue(ctx context.Context, req *domain.SandboxRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	targetKey := q.key
	if q.routing && req.NodeID != "" {
		targetKey = fmt.Sprintf("%s:%s", q.key, req.NodeID)
	}

	if err := q.client.RPush(ctx, targetKey, data).Err(); err != nil {
		return fmt.Errorf("failed to enqueue request: %w", err)
	}

	return nil
}

func (q *RedisQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, error) {
	processingKey := fmt.Sprintf("%s:processing", q.key)

	for {
		// Check context before blocking
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// BLMove atomically moves element from source (Left/Head) to destination (Left/Head of processing)
		// Available in Redis 6.2+.
		// If using older Redis, we'd need to change Enqueue to LPush and use BRPopLPush.
		// Assuming modern Redis here.
		result, err := q.client.BLMove(ctx, q.key, processingKey, "LEFT", "LEFT", 1*time.Second).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("failed to dequeue: %w", err)
		}

		var req domain.SandboxRequest
		if err := json.Unmarshal([]byte(result), &req); err != nil {
			// If we can't unmarshal, we should probably Nack it or move to dead letter.
			// For now, let's Nack it so it's not lost, but this might cause a loop.
			// Better to log and maybe move to a "corrupt" list.
			// Since we don't have a logger here, we return error.
			// But the item is now in processing list!
			// We should probably remove it from processing list if it's garbage.
			q.client.LRem(ctx, processingKey, 1, result)
			return nil, fmt.Errorf("failed to unmarshal dequeued request: %w", err)
		}

		return &req, nil
	}
}

func (q *RedisQueue) Ack(ctx context.Context, id domain.SandboxID) error {
	// We need to remove the item from the processing list.
	// Since we don't have the full item content here, we have a problem if we only have ID.
	// However, the caller (Agent) has the full request object usually.
	// But the interface only takes ID.
	// This is a limitation of the interface.
	// To fix this properly, we should either:
	// 1. Change Ack to take the full request or the raw string.
	// 2. Store items in processing list as just IDs? No, then we lose the data.
	// 3. Scan the processing list for the item with the ID.

	// Option 3 is expensive (O(N)).
	// Option 1 requires interface change.

	// Let's look at the Queue interface again.
	// Ack(ctx context.Context, id domain.SandboxID) error

	// If we change the interface, we break other implementations (MemoryQueue).
	// But MemoryQueue is easy to update.
	// Let's scan for now, assuming processing list is small (equal to concurrency of agent).
	// The agent processes one by one (or limited concurrency).

	processingKey := fmt.Sprintf("%s:processing", q.key)

	// Get all items in processing list
	items, err := q.client.LRange(ctx, processingKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to list processing items: %w", err)
	}

	for _, item := range items {
		var req domain.SandboxRequest
		if err := json.Unmarshal([]byte(item), &req); err != nil {
			continue
		}

		if req.ID == id {
			// Found it, remove it.
			// LRem removes the first occurrence of 'item'.
			if err := q.client.LRem(ctx, processingKey, 1, item).Err(); err != nil {
				return fmt.Errorf("failed to remove item from processing: %w", err)
			}
			return nil
		}
	}

	return nil // Not found, maybe already acked or expired?
}

func (q *RedisQueue) Nack(ctx context.Context, id domain.SandboxID, reason string) error {
	// Move from processing back to queue.
	// Similar to Ack, we need to find the item first.

	processingKey := fmt.Sprintf("%s:processing", q.key)

	items, err := q.client.LRange(ctx, processingKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to list processing items: %w", err)
	}

	for _, item := range items {
		var req domain.SandboxRequest
		if err := json.Unmarshal([]byte(item), &req); err != nil {
			continue
		}

		if req.ID == id {
			// Found it. Move it back.
			// We can use LMove if we want to be atomic, but LMove works on ends of lists.
			// Here we are picking a specific item from the middle.
			// So we have to: Remove from processing, Push to queue.
			// This is NOT atomic. If we crash in between, we lose the item.
			// But since we are Nacking, we are already in a failure scenario.

			// Transaction (Pipeline) can help?
			// Yes, MULTI/EXEC.

			pipe := q.client.TxPipeline()
			pipe.LRem(ctx, processingKey, 1, item)
			pipe.RPush(ctx, q.key, item) // Push to tail (retry later)
			_, err := pipe.Exec(ctx)
			if err != nil {
				return fmt.Errorf("failed to nack item: %w", err)
			}
			return nil
		}
	}

	return nil
}
