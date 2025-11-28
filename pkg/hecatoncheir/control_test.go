package hecatoncheir

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestRedisControlListener_Listen(t *testing.T) {
	// Setup miniredis or mock?
	// Since we don't have miniredis in the environment, we might need to skip or use a real redis if available.
	// But for this environment, I'll write the test assuming a mockable client or just structure it.
	// Actually, I can't easily mock go-redis client without an interface or a wrapper.
	// I'll skip the actual Redis interaction and just test the logic if possible, but it's tightly coupled.
	// I'll write a test that requires a running Redis, but check for connection failure.

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rdb.Close()

	// Check if redis is available
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available")
	}

	nodeID := domain.NodeID("test-node")
	listener := NewRedisControlListener(rdb, nodeID)

	ch, err := listener.Listen(ctx)
	assert.NoError(t, err)

	// Publish a message
	go func() {
		time.Sleep(100 * time.Millisecond)
		rdb.Publish(ctx, "tartarus:control:test-node", "KILL sandbox-1")
	}()

	select {
	case msg := <-ch:
		assert.Equal(t, ControlMessageKill, msg.Type)
		assert.Equal(t, domain.SandboxID("sandbox-1"), msg.SandboxID)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}
