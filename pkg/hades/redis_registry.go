package hades

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type RedisRegistry struct {
	client *redis.Client
}

func NewRedisRegistry(addr string, db int, password string) (*RedisRegistry, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisRegistry{client: client}, nil
}

func (r *RedisRegistry) ListNodes(ctx context.Context) ([]domain.NodeStatus, error) {
	var nodes []domain.NodeStatus
	iter := r.client.Scan(ctx, 0, "tartarus:node:*", 0).Iterator()

	for iter.Next(ctx) {
		key := iter.Val()
		val, err := r.client.Get(ctx, key).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue // Key expired during iteration
			}
			return nil, fmt.Errorf("failed to get node key %s: %w", key, err)
		}

		status, _, err := domain.UnmarshalEnvelope[domain.NodeStatus]([]byte(val), "NodeStatus")
		if err != nil {
			// Log error but continue? For now, maybe skip corrupt entries
			continue
		}
		nodes = append(nodes, status)
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan nodes: %w", err)
	}

	return nodes, nil
}

func (r *RedisRegistry) GetNode(ctx context.Context, id domain.NodeID) (*domain.NodeStatus, error) {
	key := fmt.Sprintf("tartarus:node:%s", id)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	status, _, err := domain.UnmarshalEnvelope[domain.NodeStatus]([]byte(val), "NodeStatus")
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal node status envelope: %w", err)
	}

	return &status, nil
}

func (r *RedisRegistry) UpdateHeartbeat(ctx context.Context, payload HeartbeatPayload) error {
	status := domain.NodeStatus{
		NodeInfo:        payload.Node,
		Allocated:       payload.Load,
		ActiveSandboxes: payload.ActiveSandboxes,
		Heartbeat:       payload.Time,
	}

	data, err := domain.MarshalEnvelope("NodeStatus", domain.SchemaVersionNodeStatus, status)
	if err != nil {
		return fmt.Errorf("failed to marshal node status envelope: %w", err)
	}

	key := fmt.Sprintf("tartarus:node:%s", status.ID)
	// Set with TTL
	if err := r.client.Set(ctx, key, data, NodeTTL).Err(); err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	return nil
}

func (r *RedisRegistry) MarkDraining(ctx context.Context, id domain.NodeID) error {
	// We need to get, update, and set (optimistic locking would be better, but simple get/set for now)
	// Or use a Lua script for atomicity. Given constraints, let's try a simple approach first,
	// but since we are overwriting the whole object, we should be careful.
	// Actually, UpdateHeartbeat overwrites everything too.
	// If we just want to update a label, we should probably fetch, modify, save.

	// WATCH key
	key := fmt.Sprintf("tartarus:node:%s", id)

	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		val, err := tx.Get(ctx, key).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return ErrNodeNotFound
			}
			return err
		}

		status, _, err := domain.UnmarshalEnvelope[domain.NodeStatus]([]byte(val), "NodeStatus")
		if err != nil {
			return err
		}

		if status.Labels == nil {
			status.Labels = make(map[string]string)
		}
		status.Labels["status"] = "draining"

		data, err := domain.MarshalEnvelope("NodeStatus", domain.SchemaVersionNodeStatus, status)
		if err != nil {
			return err
		}

		// Use Pipelined to execute the transaction
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, data, redis.KeepTTL)
			return nil
		})
		return err
	}, key)

	if err != nil {
		return fmt.Errorf("failed to mark draining: %w", err)
	}

	return nil
}

func (r *RedisRegistry) UpdateRun(ctx context.Context, run domain.SandboxRun) error {
	data, err := domain.MarshalEnvelope("SandboxRun", domain.SchemaVersionSandboxRun, run)
	if err != nil {
		return fmt.Errorf("failed to marshal run envelope: %w", err)
	}

	key := fmt.Sprintf("tartarus:run:%s", run.ID)
	// Store run indefinitely (or with long TTL)
	if err := r.client.Set(ctx, key, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to update run: %w", err)
	}

	return nil
}

func (r *RedisRegistry) GetRun(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	key := fmt.Sprintf("tartarus:run:%s", id)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	run, _, err := domain.UnmarshalEnvelope[domain.SandboxRun]([]byte(val), "SandboxRun")
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal run envelope: %w", err)
	}

	return &run, nil
}

func (r *RedisRegistry) ListRuns(ctx context.Context) ([]domain.SandboxRun, error) {
	var runs []domain.SandboxRun
	iter := r.client.Scan(ctx, 0, "tartarus:run:*", 0).Iterator()

	for iter.Next(ctx) {
		key := iter.Val()
		val, err := r.client.Get(ctx, key).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue // Key expired during iteration
			}
			return nil, fmt.Errorf("failed to get run key %s: %w", key, err)
		}

		run, _, err := domain.UnmarshalEnvelope[domain.SandboxRun]([]byte(val), "SandboxRun")
		if err != nil {
			// Log error but continue? For now, maybe skip corrupt entries
			continue
		}
		runs = append(runs, run)
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan runs: %w", err)
	}

	return runs, nil
}

// MigrateSchema iterates over all node and run keys, and upgrades them to the latest schema envelope.
func (r *RedisRegistry) MigrateSchema(ctx context.Context) error {
	// Migrate Nodes
	nodeIter := r.client.Scan(ctx, 0, "tartarus:node:*", 0).Iterator()
	for nodeIter.Next(ctx) {
		key := nodeIter.Val()
		val, err := r.client.Get(ctx, key).Result()
		if err != nil {
			continue // Skip
		}

		status, version, err := domain.UnmarshalEnvelope[domain.NodeStatus]([]byte(val), "NodeStatus")
		if err != nil {
			continue // Skip
		}

		if version < domain.SchemaVersionNodeStatus {
			data, err := domain.MarshalEnvelope("NodeStatus", domain.SchemaVersionNodeStatus, status)
			if err == nil {
				r.client.Set(ctx, key, data, redis.KeepTTL)
			}
		}
	}

	// Migrate Runs
	runIter := r.client.Scan(ctx, 0, "tartarus:run:*", 0).Iterator()
	for runIter.Next(ctx) {
		key := runIter.Val()
		val, err := r.client.Get(ctx, key).Result()
		if err != nil {
			continue // Skip
		}

		run, version, err := domain.UnmarshalEnvelope[domain.SandboxRun]([]byte(val), "SandboxRun")
		if err != nil {
			continue // Skip
		}

		if version < domain.SchemaVersionSandboxRun {
			data, err := domain.MarshalEnvelope("SandboxRun", domain.SchemaVersionSandboxRun, run)
			if err == nil {
				r.client.Set(ctx, key, data, redis.KeepTTL)
			}
		}
	}

	return nil
}
