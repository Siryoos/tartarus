package olympus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
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
	topic := fmt.Sprintf("tartarus:control:%s", nodeID)
	// Format: EXEC sandboxID cmd...
	// We need to join cmd args carefully or use JSON?
	// The agent parses space-separated args.
	// "TYPE SANDBOX_ID [ARGS...]"
	// If cmd contains spaces, simple split will break.
	// But for now, let's assume simple args or we need to change agent parsing.
	// Agent uses strings.Split(msg.Payload, " ").
	// This is fragile for Exec.
	// But to keep it simple and consistent with existing protocol:
	// We can join with spaces.
	// If we want robust exec, we should use JSON payload for control messages.
	// But that requires changing all messages.
	// For this task, let's just join with spaces and note the limitation.
	// Or we can encode the cmd args as a single JSON string argument?
	// Agent: args = parts[2:] -> this takes all remaining parts.
	// So if we send "EXEC id arg1 arg2", agent gets ["arg1", "arg2"].
	// This works for simple commands.
	// If arg has spaces "arg with space", agent gets ["arg", "with", "space"].
	// This breaks arguments with spaces.
	// However, changing the protocol is out of scope for "CLI v2.0" unless necessary.
	// Let's stick to simple args for now as per "CLI v2.0" usually implies basic functionality first.
	// Or better: use a separator that is unlikely? No.
	// Let's just join with spaces.

	msg := fmt.Sprintf("EXEC %s", sandboxID)
	for _, arg := range cmd {
		msg += " " + arg
	}
	return r.client.Publish(ctx, topic, msg).Err()
}

func (r *RedisControlPlane) ListSandboxes(ctx context.Context, nodeID domain.NodeID) ([]domain.SandboxRun, error) {
	requestID := uuid.New().String()
	responseTopic := fmt.Sprintf("tartarus:response:%s", requestID)

	// 1. Subscribe to response topic
	pubsub := r.client.Subscribe(ctx, responseTopic)
	defer pubsub.Close()

	// Verify subscription
	if _, err := pubsub.Receive(ctx); err != nil {
		return nil, fmt.Errorf("failed to subscribe to response topic: %w", err)
	}

	// 2. Send request
	controlTopic := fmt.Sprintf("tartarus:control:%s", nodeID)
	msg := fmt.Sprintf("LIST_SANDBOXES %s", requestID)
	if err := r.client.Publish(ctx, controlTopic, msg).Err(); err != nil {
		return nil, fmt.Errorf("failed to send list request: %w", err)
	}

	// 3. Wait for response with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ch := pubsub.Channel()
	select {
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timeout waiting for agent response")
	case msg := <-ch:
		var runs []domain.SandboxRun
		if err := json.Unmarshal([]byte(msg.Payload), &runs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return runs, nil
	}
}
