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

func (r *RedisControlPlane) Exec(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	requestID := uuid.New().String()
	responseTopic := fmt.Sprintf("tartarus:exec:%s:%s", sandboxID, requestID)

	// 1. Subscribe to response topic
	pubsub := r.client.Subscribe(ctx, responseTopic)
	defer pubsub.Close()

	// Verify subscription
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to exec output: %w", err)
	}

	// 2. Send request
	topic := fmt.Sprintf("tartarus:control:%s", nodeID)
	// Format: EXEC sandboxID requestID args...
	msg := fmt.Sprintf("EXEC %s %s", sandboxID, requestID)
	for _, arg := range cmd {
		msg += " " + arg
	}
	if err := r.client.Publish(ctx, topic, msg).Err(); err != nil {
		return fmt.Errorf("failed to send exec command: %w", err)
	}

	// 3. Stream output
	ch := pubsub.Channel()
	// Use a timeout for inactivity? Or just context?
	// If the command hangs, the user can cancel the context.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			// Write to stdout (we don't distinguish stdout/stderr in this simple protocol yet)
			if _, err := stdout.Write([]byte(msg.Payload)); err != nil {
				return err
			}
		}
	}
}

func (r *RedisControlPlane) ExecInteractive(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	requestID := uuid.New().String()
	responseTopic := fmt.Sprintf("tartarus:exec:%s:%s", sandboxID, requestID)
	stdinTopic := fmt.Sprintf("tartarus:exec:stdin:%s", requestID)

	// 1. Subscribe to response topic (stdout/stderr)
	pubsub := r.client.Subscribe(ctx, responseTopic)
	defer pubsub.Close()

	// Verify subscription
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to exec output: %w", err)
	}

	// 2. Send request
	topic := fmt.Sprintf("tartarus:control:%s", nodeID)
	// Format: EXEC_INTERACTIVE sandboxID requestID args...
	msg := fmt.Sprintf("EXEC_INTERACTIVE %s %s", sandboxID, requestID)
	for _, arg := range cmd {
		msg += " " + arg
	}
	if err := r.client.Publish(ctx, topic, msg).Err(); err != nil {
		return fmt.Errorf("failed to send exec command: %w", err)
	}

	// 3. Stream stdin in background
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				if err := r.client.Publish(ctx, stdinTopic, buf[:n]).Err(); err != nil {
					// Failed to publish stdin, maybe context canceled
					return
				}
			}
			if err != nil {
				// EOF or error
				return
			}
		}
	}()

	// 4. Stream output
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			// Write to stdout (we don't distinguish stdout/stderr in this simple protocol yet)
			if _, err := stdout.Write([]byte(msg.Payload)); err != nil {
				return err
			}
		}
	}
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
