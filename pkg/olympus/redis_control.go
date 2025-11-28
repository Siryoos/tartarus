package olympus

import (
	"context"
	"fmt"
	"io"

	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type RedisControlPlane struct {
	client *redis.Client
}

func NewRedisControlPlane(client *redis.Client) *RedisControlPlane {
	return &RedisControlPlane{client: client}
}

func (r *RedisControlPlane) Kill(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	topic := fmt.Sprintf("tartarus:control:%s", nodeID)
	msg := fmt.Sprintf("KILL %s", sandboxID)
	return r.client.Publish(ctx, topic, msg).Err()
}

func (r *RedisControlPlane) StreamLogs(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, w io.Writer) error {
	// 1. Subscribe to logs FIRST to avoid race condition
	logsTopic := fmt.Sprintf("tartarus:logs:%s", sandboxID)
	pubsub := r.client.Subscribe(ctx, logsTopic)
	// Verify subscription
	if _, err := pubsub.Receive(ctx); err != nil {
		pubsub.Close()
		return err
	}
	defer pubsub.Close()

	// 2. Trigger agent to start streaming
	controlTopic := fmt.Sprintf("tartarus:control:%s", nodeID)
	msg := fmt.Sprintf("LOGS %s", sandboxID)
	if err := r.client.Publish(ctx, controlTopic, msg).Err(); err != nil {
		return err
	}

	// 3. Stream logs to writer

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-ch:
			if _, err := w.Write([]byte(msg.Payload)); err != nil {
				return err
			}
		}
	}
}
