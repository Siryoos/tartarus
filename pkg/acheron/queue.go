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
	// Len returns the current queue depth for metrics/scaling decisions.
	Len(ctx context.Context) int
}

// NumPriorityLevels is the number of distinct scheduling tiers supported by
// PriorityQueue.  It must equal int(domain.PriorityHigh) + 1.
// Update this constant whenever new domain.Priority constants are added.
const NumPriorityLevels = int(domain.PriorityHigh) + 1 // == 3

