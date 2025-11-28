package acheron

import (
	"context"
	"sync"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type MemoryQueue struct {
	mu    sync.Mutex
	items []*domain.SandboxRequest
	cond  *sync.Cond
}

func NewMemoryQueue() *MemoryQueue {
	q := &MemoryQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *MemoryQueue) Enqueue(ctx context.Context, req *domain.SandboxRequest) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, req)
	q.cond.Signal()
	return nil
}

func (q *MemoryQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 {
		// In a real implementation, we would respect context cancellation here.
		// For this simple sync.Cond implementation, we just wait.
		// To properly handle context, we'd need a channel-based approach or polling.
		// For this prototype, checking context before waiting is a partial fix.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		q.cond.Wait()
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	item := q.items[0]
	q.items = q.items[1:]
	return item, nil
}

func (q *MemoryQueue) Ack(ctx context.Context, id domain.SandboxID) error {
	// No-op for memory queue
	return nil
}

func (q *MemoryQueue) Nack(ctx context.Context, id domain.SandboxID, reason string) error {
	// Simple Nack: just log it (or in a real system, maybe re-queue)
	return nil
}
