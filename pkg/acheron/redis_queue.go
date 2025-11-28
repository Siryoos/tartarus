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
	client *redis.Client
	key    string
}

func NewRedisQueue(addr string, db int, key string) (*RedisQueue, error) {
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
		client: client,
		key:    key,
	}, nil
}

func (q *RedisQueue) Enqueue(ctx context.Context, req *domain.SandboxRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	if err := q.client.RPush(ctx, q.key, data).Err(); err != nil {
		return fmt.Errorf("failed to enqueue request: %w", err)
	}

	return nil
}

func (q *RedisQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, error) {
	for {
		// Check context before blocking
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// BLPOP with a short timeout to allow checking context cancellation
		// We use 1 second timeout.
		result, err := q.client.BLPop(ctx, 1*time.Second, q.key).Result()
		if err != nil {
			if err == redis.Nil {
				// Timeout, loop again to check context
				continue
			}
			// If context was canceled during BLPop, redis client might return an error related to that
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("failed to dequeue: %w", err)
		}

		// result[0] is the key, result[1] is the value
		if len(result) < 2 {
			continue
		}

		var req domain.SandboxRequest
		if err := json.Unmarshal([]byte(result[1]), &req); err != nil {
			// If we can't unmarshal, we probably shouldn't crash, but we also can't process it.
			// For now, let's return an error so the caller knows something went wrong.
			// In a robust system, we might move this to a dead-letter queue.
			return nil, fmt.Errorf("failed to unmarshal dequeued request: %w", err)
		}

		return &req, nil
	}
}

func (q *RedisQueue) Ack(ctx context.Context, id domain.SandboxID) error {
	// No-op for simple list-based queue as the item is already popped.
	return nil
}

func (q *RedisQueue) Nack(ctx context.Context, id domain.SandboxID, reason string) error {
	// For now, we just log that we are Nacking.
	// In the future, we could push to a dead-letter list.
	// Since we don't have a logger here, we'll just return nil.
	return nil
}
