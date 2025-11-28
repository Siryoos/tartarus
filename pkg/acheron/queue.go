package acheron

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Queue is Acheron: the river all jobs must cross to reach Tartarus.

type Queue interface {
	Enqueue(ctx context.Context, req *domain.SandboxRequest) error
	Dequeue(ctx context.Context) (*domain.SandboxRequest, error)
	Ack(ctx context.Context, id domain.SandboxID) error
	Nack(ctx context.Context, id domain.SandboxID, reason string) error
}
