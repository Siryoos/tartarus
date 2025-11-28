package acheron

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Queue is Acheron: the river all jobs must cross to reach Tartarus.

type Queue interface {
	Enqueue(ctx context.Context, req *domain.SandboxRequest) error
	Dequeue(ctx context.Context) (*domain.SandboxRequest, string, error)
	Ack(ctx context.Context, receipt string) error
	Nack(ctx context.Context, receipt string, reason string) error
}
