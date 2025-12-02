package hecatoncheir

import (
	"context"
	"fmt"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// PublishExecOutput publishes exec output to a topic.
func (r *RedisControlListener) PublishExecOutput(ctx context.Context, sandboxID domain.SandboxID, requestID string, output []byte) error {
	topic := fmt.Sprintf("tartarus:exec:%s:%s", sandboxID, requestID)
	return r.client.Publish(ctx, topic, output).Err()
}
