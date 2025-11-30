package olympus

import (
	"context"
	"fmt"
	"io"
	"time"

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

func (r *RedisControlPlane) StreamLogs(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, w io.Writer, follow bool) error {
	// 1. Subscribe to logs FIRST to avoid race condition
	logsTopic := fmt.Sprintf("tartarus:logs:%s", sandboxID)
	pubsub := r.client.Subscribe(ctx, logsTopic)
	// Verify subscription
	if _, err := pubsub.Receive(ctx); err != nil {
		pubsub.Close()
		return fmt.Errorf("failed to subscribe to logs: %w", err)
	}
	defer pubsub.Close()

	// 2. Trigger agent to start streaming
	controlTopic := fmt.Sprintf("tartarus:control:%s", nodeID)
	msg := fmt.Sprintf("LOGS %s %v", sandboxID, follow)
	if err := r.client.Publish(ctx, controlTopic, msg).Err(); err != nil {
		return fmt.Errorf("failed to trigger log streaming: %w", err)
	}

	// 3. Stream logs to writer with timeout
	// Create a timeout context (5 minutes max for log streaming)
	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	ch := pubsub.Channel()

	for {
		select {
		case <-streamCtx.Done():
			// Differentiate between timeout and user cancellation
			if ctx.Err() == nil {
				// Parent context is still valid, so this was a timeout
				return fmt.Errorf("log streaming timeout after 5 minutes")
			}
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				// Channel closed, streaming ended
				return nil
			}
			if msg == nil {
				// Nil message, skip
				continue
			}
			if _, err := w.Write([]byte(msg.Payload)); err != nil {
				return fmt.Errorf("failed to write logs: %w", err)
			}
		}
	}
}

func (r *RedisControlPlane) Hibernate(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	topic := fmt.Sprintf("tartarus:control:%s", nodeID)
	msg := fmt.Sprintf("HIBERNATE %s", sandboxID)
	return r.client.Publish(ctx, topic, msg).Err()
}

func (r *RedisControlPlane) Wake(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	topic := fmt.Sprintf("tartarus:control:%s", nodeID)
	msg := fmt.Sprintf("WAKE %s", sandboxID)
	return r.client.Publish(ctx, topic, msg).Err()
}

func (r *RedisControlPlane) Snapshot(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	topic := fmt.Sprintf("tartarus:control:%s", nodeID)
	msg := fmt.Sprintf("SNAPSHOT %s", sandboxID)
	return r.client.Publish(ctx, topic, msg).Err()
}

func (r *RedisControlPlane) Exec(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, cmd []string) error {
	// Exec is not yet supported by the runtime, but we can stub it here.
	// We might want to publish an EXEC message if we were to support it.
	// For now, return "not implemented" error or just log.
	return fmt.Errorf("exec not implemented")
}
