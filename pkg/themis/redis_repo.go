package themis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// RedisRepo is a Redis-backed implementation of the Repository interface.
type RedisRepo struct {
	client *redis.Client
}

// NewRedisRepo creates a new Redis-backed policy repository.
func NewRedisRepo(addr string, db int, password string) (*RedisRepo, error) {
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

	return &RedisRepo{client: client}, nil
}

// GetPolicy retrieves a policy for the given template ID.
// If no policy exists, it returns a default lockdown policy.
func (r *RedisRepo) GetPolicy(ctx context.Context, tplID domain.TemplateID) (*domain.SandboxPolicy, error) {
	key := fmt.Sprintf("themis:policy:%s", tplID)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Return default lockdown policy
			return &domain.SandboxPolicy{
				ID:         domain.PolicyID("lockdown-default"),
				TemplateID: tplID,
				Resources: domain.ResourceSpec{
					CPU: 1000, // 1 CPU core (1000 milliCPU)
					Mem: 128,  // 128 MB
				},
				NetworkPolicy: domain.NetworkPolicyRef{
					ID:   "lockdown-no-net",
					Name: "No Internet",
				},
				Tags: map[string]string{
					"type": "default-lockdown",
				},
				Version: 0,
			}, nil
		}
		return nil, fmt.Errorf("failed to get policy: %w", err)
	}

	var policy domain.SandboxPolicy
	if err := json.Unmarshal([]byte(val), &policy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal policy: %w", err)
	}

	return &policy, nil
}

// UpsertPolicy inserts or updates a policy in the repository using optimistic locking.
func (r *RedisRepo) UpsertPolicy(ctx context.Context, p *domain.SandboxPolicy) error {
	key := fmt.Sprintf("themis:policy:%s", p.TemplateID)

	// Optimistic locking with WATCH
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		val, err := tx.Get(ctx, key).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}

		var currentVersion int64
		if err == nil {
			var existing domain.SandboxPolicy
			if err := json.Unmarshal([]byte(val), &existing); err != nil {
				return err
			}
			currentVersion = existing.Version
		}

		// Check version conflict
		// If p.Version is 0, we expect it to be a new policy (currentVersion should be 0)
		// If p.Version > 0, we expect it to match currentVersion (and we will increment it)
		// Actually, standard optimistic locking usually expects the passed object to have the *current* version,
		// and we increment it on save.
		// Or, the passed object has the *new* version, and we check if it's strictly +1.
		// Let's assume the caller passes the object they read, so p.Version is the version they saw.
		// We will save it as p.Version + 1.

		if p.Version != currentVersion {
			return fmt.Errorf("version conflict: expected %d, got %d", currentVersion, p.Version)
		}

		p.Version++

		data, err := json.Marshal(p)
		if err != nil {
			return err
		}

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, data, 0)
			return nil
		})
		return err
	}, key)

	if err != nil {
		if errors.Is(err, redis.TxFailedErr) {
			return fmt.Errorf("optimistic lock failed: %w", err)
		}
		return fmt.Errorf("failed to upsert policy: %w", err)
	}

	return nil
}

// ListPolicies returns all stored policies.
func (r *RedisRepo) ListPolicies(ctx context.Context) ([]*domain.SandboxPolicy, error) {
	var policies []*domain.SandboxPolicy
	iter := r.client.Scan(ctx, 0, "themis:policy:*", 0).Iterator()

	for iter.Next(ctx) {
		key := iter.Val()
		val, err := r.client.Get(ctx, key).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			return nil, fmt.Errorf("failed to get policy key %s: %w", key, err)
		}

		var p domain.SandboxPolicy
		if err := json.Unmarshal([]byte(val), &p); err != nil {
			continue
		}
		policies = append(policies, &p)
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan policies: %w", err)
	}

	return policies, nil
}
