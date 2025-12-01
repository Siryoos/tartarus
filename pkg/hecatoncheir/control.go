package hecatoncheir

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// ControlMessageType defines the type of control message.
type ControlMessageType string

const (
	ControlMessageKill          ControlMessageType = "KILL"
	ControlMessageLogs          ControlMessageType = "LOGS"
	ControlMessageHibernate     ControlMessageType = "HIBERNATE"
	ControlMessageWake          ControlMessageType = "WAKE"
	ControlMessageTerminate     ControlMessageType = "TERMINATE"
	ControlMessageSnapshot      ControlMessageType = "SNAPSHOT"
	ControlMessageExec          ControlMessageType = "EXEC"
	ControlMessageListSandboxes ControlMessageType = "LIST_SANDBOXES"
)

// ControlMessage is a command sent to the agent.
type ControlMessage struct {
	Type      ControlMessageType
	SandboxID domain.SandboxID
	Args      []string
}

// ControlListener listens for control messages.
type ControlListener interface {
	// Listen returns a channel of control messages.
	Listen(ctx context.Context) (<-chan ControlMessage, error)
	// PublishLogs publishes log chunks to a topic.
	PublishLogs(ctx context.Context, sandboxID domain.SandboxID, logs []byte) error
	// PublishSandboxes publishes the list of sandboxes to a response topic.
	PublishSandboxes(ctx context.Context, requestID string, sandboxes []domain.SandboxRun) error
}

// RedisControlListener implements ControlListener using Redis Pub/Sub.
type RedisControlListener struct {
	client *redis.Client
	nodeID domain.NodeID
}

// NewRedisControlListener creates a new RedisControlListener.
func NewRedisControlListener(client *redis.Client, nodeID domain.NodeID) *RedisControlListener {
	return &RedisControlListener{
		client: client,
		nodeID: nodeID,
	}
}

// Listen subscribes to the node's control topic and returns a channel of messages.
func (r *RedisControlListener) Listen(ctx context.Context) (<-chan ControlMessage, error) {
	topic := fmt.Sprintf("tartarus:control:%s", r.nodeID)
	pubsub := r.client.Subscribe(ctx, topic)

	// Verify connection
	if _, err := pubsub.Receive(ctx); err != nil {
		return nil, err
	}

	ch := make(chan ControlMessage)

	go func() {
		defer close(ch)
		defer pubsub.Close()

		redisCh := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-redisCh:
				if !ok {
					return
				}
				// Parse message: "TYPE SANDBOX_ID [ARGS...]"
				parts := strings.Split(msg.Payload, " ")
				if len(parts) < 2 {
					continue
				}

				cmdType := ControlMessageType(parts[0])
				sandboxID := domain.SandboxID(parts[1])
				var args []string
				if len(parts) > 2 {
					args = parts[2:]
				}

				ch <- ControlMessage{
					Type:      cmdType,
					SandboxID: sandboxID,
					Args:      args,
				}
			}
		}
	}()

	return ch, nil
}

// PublishLogs publishes log chunks to the sandbox's log topic.
func (r *RedisControlListener) PublishLogs(ctx context.Context, sandboxID domain.SandboxID, logs []byte) error {
	topic := fmt.Sprintf("tartarus:logs:%s", sandboxID)
	return r.client.Publish(ctx, topic, logs).Err()
}

// PublishSandboxes publishes the list of sandboxes to a response topic.
func (r *RedisControlListener) PublishSandboxes(ctx context.Context, requestID string, sandboxes []domain.SandboxRun) error {
	topic := fmt.Sprintf("tartarus:response:%s", requestID)
	payload, err := json.Marshal(sandboxes)
	if err != nil {
		return err
	}
	return r.client.Publish(ctx, topic, payload).Err()
}
