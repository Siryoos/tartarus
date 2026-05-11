package acheron

import (
	"context"
	"fmt"
	"sync"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// MemoryQueue is an in-memory implementation of Queue for testing and
// single-node development deployments. It does not persist across restarts.
//
// Dequeue correctly respects context cancellation by using a channel-based
// notification pattern instead of sync.Cond, allowing clean graceful shutdown
// even when the queue is idle.
type MemoryQueue struct {
	mu         sync.Mutex
	items      []*domain.SandboxRequest
	processing map[string]*domain.SandboxRequest // O(1) lookup for Ack/Nack
	notify     chan struct{}                       // closed to wake Dequeue waiters
	nextID     int                                // for generating receipt IDs
	metrics    hermes.Metrics
	queueName  string
}

// NewMemoryQueue creates a new in-memory queue with the given metrics sink.
// Pass hermes.NewNoopMetrics() for test/development code that does not need
// Prometheus instrumentation.
func NewMemoryQueue(metrics hermes.Metrics, queueName string) *MemoryQueue {
	return &MemoryQueue{
		processing: make(map[string]*domain.SandboxRequest),
		notify:     make(chan struct{}),
		metrics:    metrics,
		queueName:  queueName,
	}
}

// Enqueue adds a request to the tail of the queue and wakes any blocked Dequeue
// callers. O(1) amortised.
func (q *MemoryQueue) Enqueue(ctx context.Context, req *domain.SandboxRequest) error {
	q.mu.Lock()
	q.items = append(q.items, req)
	old := q.notify
	q.notify = make(chan struct{})
	depth := len(q.items) + len(q.processing)
	q.mu.Unlock()

	// Wake blocked Dequeue callers by closing the old channel.
	close(old)

	q.metrics.IncCounter("queue_enqueue_total", 1, hermes.Label{Key: "queue", Value: q.queueName})
	q.metrics.SetGauge("queue_depth", float64(depth), hermes.Label{Key: "queue", Value: q.queueName})
	return nil
}

// Dequeue blocks until an item is available or the context is cancelled.
// Unlike a sync.Cond-based implementation, cancellation is honoured
// immediately even while the goroutine is parked waiting for work.
func (q *MemoryQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, string, error) {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			item := q.items[0]
			q.items = q.items[1:]

			q.nextID++
			receipt := fmt.Sprintf("mem-receipt-%d", q.nextID)
			q.processing[receipt] = item
			depth := len(q.items) + len(q.processing)
			q.mu.Unlock()

			q.metrics.IncCounter("queue_dequeue_total", 1, hermes.Label{Key: "queue", Value: q.queueName})
			q.metrics.SetGauge("queue_depth", float64(depth), hermes.Label{Key: "queue", Value: q.queueName})
			return item, receipt, nil
		}
		// Capture the current notify channel while holding the lock so we
		// don't miss a signal that arrives between the unlock and the select.
		ch := q.notify
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-ch:
			// An item was enqueued or re-queued; loop back and check.
		}
	}
}

// Ack removes an item from the in-flight processing map. O(1).
func (q *MemoryQueue) Ack(ctx context.Context, receipt string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.processing, receipt) // silent no-op if already gone, matching Redis XACK behaviour
	return nil
}

// Nack re-queues an item at the tail and wakes any blocked Dequeue callers. O(1).
func (q *MemoryQueue) Nack(ctx context.Context, receipt string, reason string) error {
	q.mu.Lock()
	item, exists := q.processing[receipt]
	if !exists {
		q.mu.Unlock()
		return nil // silent no-op to match Redis XACK behaviour
	}
	q.items = append(q.items, item)
	delete(q.processing, receipt)
	old := q.notify
	q.notify = make(chan struct{})
	q.mu.Unlock()

	close(old)

	q.metrics.IncCounter("queue_nack_total", 1, hermes.Label{Key: "queue", Value: q.queueName})
	return nil
}

// errQueueEmpty is returned by dequeueNonBlocking when the queue has no items.
var errQueueEmpty = fmt.Errorf("queue empty")

// pendingLen returns only the count of pending (not yet dequeued) items.
// Used by PriorityQueue to check a tier without acquiring the outer lock.
func (q *MemoryQueue) pendingLen() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// dequeueNonBlocking returns an item immediately if one is available, or
// errQueueEmpty if the queue is empty.  Never blocks.
// Used internally by PriorityQueue to drain a tier without waiting.
func (q *MemoryQueue) dequeueNonBlocking() (*domain.SandboxRequest, string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil, "", errQueueEmpty
	}
	item := q.items[0]
	q.items = q.items[1:]
	q.nextID++
	receipt := fmt.Sprintf("mem-receipt-%d", q.nextID)
	q.processing[receipt] = item
	return item, receipt, nil
}

// Len returns the current queue depth (pending + in-flight). O(1).
func (q *MemoryQueue) Len(ctx context.Context) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) + len(q.processing)
}
