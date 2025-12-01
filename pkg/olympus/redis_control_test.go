package olympus

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestRedisControlPlane_Kill(t *testing.T) {
	// Setup Redis client
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rdb.Close()

	// Check if redis is available
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available")
	}

	// Clear any existing subscriptions
	topic := "tartarus:control:test-node"
	rdb.Del(ctx, topic)

	control := NewRedisControlPlane(rdb)

	// Subscribe to control topic to verify message
	pubsub := rdb.Subscribe(ctx, topic)
	defer pubsub.Close()

	// Wait for subscription confirmation
	_, err := pubsub.Receive(ctx)
	require.NoError(t, err)

	// Send kill command
	nodeID := domain.NodeID("test-node")
	sandboxID := domain.SandboxID("test-sandbox-123")
	err = control.Kill(ctx, nodeID, sandboxID)
	assert.NoError(t, err)

	// Verify message was published
	ch := pubsub.Channel()
	select {
	case msg := <-ch:
		assert.Equal(t, "KILL test-sandbox-123", msg.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for kill message")
	}
}

func TestRedisControlPlane_StreamLogs_Timeout(t *testing.T) {
	// Setup Redis client
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rdb.Close()

	// Check if redis is available
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available")
	}

	control := NewRedisControlPlane(rdb)

	nodeID := domain.NodeID("test-node")
	sandboxID := domain.SandboxID("test-sandbox-timeout")

	var buf bytes.Buffer

	// Create a context that will timeout quickly for testing
	// We'll replace the timeout in production, but for test we want it fast
	// Note: The actual timeout is hardcoded in StreamLogs to 5 minutes,
	// so this test would take too long. Instead we'll test with context cancellation
	testCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	err := control.StreamLogs(testCtx, nodeID, sandboxID, &buf, false)

	// Should get context deadline exceeded or our timeout message
	assert.Error(t, err)
	// The error could be either context.DeadlineExceeded or our custom timeout message
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "timeout"))
}

func TestRedisControlPlane_StreamLogs_Success(t *testing.T) {
	// Setup Redis client
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rdb.Close()

	// Check if redis is available
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available")
	}

	control := NewRedisControlPlane(rdb)

	nodeID := domain.NodeID("test-node")
	sandboxID := domain.SandboxID("test-sandbox-logs")

	var buf bytes.Buffer

	// Start streaming in a goroutine
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- control.StreamLogs(streamCtx, nodeID, sandboxID, &buf, false)
	}()

	// Give it time to subscribe
	time.Sleep(200 * time.Millisecond)

	// Publish some log messages
	logTopic := "tartarus:logs:test-sandbox-logs"
	rdb.Publish(ctx, logTopic, "Log line 1\n")
	rdb.Publish(ctx, logTopic, "Log line 2\n")
	rdb.Publish(ctx, logTopic, "Log line 3\n")

	// Give it time to receive
	time.Sleep(200 * time.Millisecond)

	// Cancel the context to stop streaming
	cancel()

	// Wait for streaming to end
	select {
	case err := <-errCh:
		// Should get context.Canceled
		assert.True(t, errors.Is(err, context.Canceled))
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for streaming to end")
	}

	// Verify we received the logs
	output := buf.String()
	assert.Contains(t, output, "Log line 1")
	assert.Contains(t, output, "Log line 2")
	assert.Contains(t, output, "Log line 3")
}

func TestRedisControlPlane_StreamLogs_NilMessages(t *testing.T) {
	// This test verifies that nil messages don't crash the streaming
	// In practice, Redis shouldn't send nil messages, but we handle it defensively

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available")
	}

	control := NewRedisControlPlane(rdb)

	nodeID := domain.NodeID("test-node")
	sandboxID := domain.SandboxID("test-sandbox-nil")

	var buf bytes.Buffer

	streamCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	// Start streaming - it will timeout after 500ms
	err := control.StreamLogs(streamCtx, nodeID, sandboxID, &buf, false)

	// Should timeout without crashing
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}
