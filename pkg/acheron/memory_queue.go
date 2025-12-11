package acheron

import (
	"context"
	"fmt"
	"sync"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// MemoryQueue is an in-memory implementation of Queue for testing.
// It maintains O(1) Ack/Nack operations using a processing map,
// matching the performance characteristics of RedisQueue.
type MemoryQueue struct {
	mu         sync.Mutex
	items      []*domain.SandboxRequest
	processing map[string]*domain.SandboxRequest // O(1) lookup for Ack/Nack
	cond       *sync.Cond
	nextID     int // For generating receipt IDs
}

func NewMemoryQueue() *MemoryQueue {
	q := &MemoryQueue{
		processing: make(map[string]*domain.SandboxRequest),
	}
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

func (q *MemoryQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 {
		// In a real implementation, we would respect context cancellation here.
		// For this simple sync.Cond implementation, we just wait.
		// To properly handle context, we'd need a channel-based approach or polling.
		// For this prototype, checking context before waiting is a partial fix.
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		q.cond.Wait()
	}

	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}

	item := q.items[0]
	q.items = q.items[1:]

	// Generate receipt and track in processing map
	q.nextID++
	receipt := fmt.Sprintf("receipt-%d", q.nextID)
	q.processing[receipt] = item

	return item, receipt, nil
}

// Ack removes an item from the processing map.
// This is O(1) hash map deletion, matching RedisQueue's XACK performance.
func (q *MemoryQueue) Ack(ctx context.Context, receipt string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.processing[receipt]; !exists {
		// Silent failure to match Redis behavior (XACK returns count, doesn't error)
		return nil
	}

	delete(q.processing, receipt)
	return nil
}

// Nack re-queues an item and removes it from processing.
// This is O(1) for the map operation, O(1) for the append.
func (q *MemoryQueue) Nack(ctx context.Context, receipt string, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.processing[receipt]
	if !exists {
		// Silent failure to match Redis behavior
		return nil
	}

	// Re-enqueue at the end
	q.items = append(q.items, item)
	delete(q.processing, receipt)
	q.cond.Signal()

	return nil
}

// Len returns the current queue depth (pending + processing).
func (q *MemoryQueue) Len(ctx context.Context) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) + len(q.processing)
}
