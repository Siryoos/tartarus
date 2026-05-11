package acheron

import (
	"context"
	"fmt"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// PriorityQueue is a multi-level FIFO queue that dequeues higher-priority
// items before lower-priority ones.  Internally it maintains one MemoryQueue
// per priority level (indexed 0 = low … NumPriorityLevels-1 = high) and uses
// a channel-based fan-in so that Dequeue blocks correctly while still
// honouring context cancellation.
//
// Priority levels map to domain.Priority constants:
//
//	level 0 → domain.PriorityLow
//	level 1 → domain.PriorityNormal
//	level 2 → domain.PriorityHigh
//
// Requests with a Priority value outside the valid range are clamped to
// PriorityNormal.
type PriorityQueue struct {
	levels    []*MemoryQueue  // index == priority level; highest index wins
	ready     chan int         // signals which level has a new item; buffered
	metrics   hermes.Metrics
	queueName string
}

// NewPriorityQueue creates a PriorityQueue with NumPriorityLevels tiers.
// The metrics sink and queueName label are forwarded to every tier so the
// aggregate counters are visible under a single queue name in dashboards.
func NewPriorityQueue(metrics hermes.Metrics, queueName string) *PriorityQueue {
	levels := make([]*MemoryQueue, NumPriorityLevels)
	// Each sub-queue gets a noop metrics sink; the PriorityQueue itself
	// handles aggregated instrumentation via the shared metrics sink.
	noop := hermes.NewNoopMetrics()
	for i := range levels {
		levels[i] = NewMemoryQueue(noop, "")
	}
	return &PriorityQueue{
		levels:    levels,
		ready:     make(chan int, NumPriorityLevels*16), // generous buffer to avoid drops
		metrics:   metrics,
		queueName: queueName,
	}
}

// clampPriority maps any domain.Priority value to a valid level index.
func clampPriority(p domain.Priority) int {
	lvl := int(p)
	if lvl < 0 {
		return 0
	}
	if lvl >= NumPriorityLevels {
		return NumPriorityLevels - 1
	}
	return lvl
}

// Enqueue places the request in the sub-queue for its priority tier and
// signals Dequeue waiters via the ready channel.
func (pq *PriorityQueue) Enqueue(ctx context.Context, req *domain.SandboxRequest) error {
	lvl := clampPriority(req.Priority)
	if err := pq.levels[lvl].Enqueue(ctx, req); err != nil {
		pq.metrics.IncCounter("queue_enqueue_errors_total", 1, hermes.Label{Key: "queue", Value: pq.queueName})
		return err
	}
	// Signal Dequeue; non-blocking send so a full buffer doesn't deadlock.
	// Dequeue will drain the highest-priority tier first, so the exact level
	// sent here is advisory only.
	select {
	case pq.ready <- lvl:
	default:
	}
	pq.metrics.IncCounter("queue_enqueue_total", 1, hermes.Label{Key: "queue", Value: pq.queueName})
	pq.metrics.SetGauge("queue_depth", float64(pq.Len(ctx)), hermes.Label{Key: "queue", Value: pq.queueName})
	return nil
}

// Dequeue blocks until a request is available or ctx is cancelled.
// Items are always returned from the highest non-empty priority tier first,
// maintaining strict priority ordering.
func (pq *PriorityQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, string, error) {
	for {
		// Always scan from highest priority downward.
		for lvl := NumPriorityLevels - 1; lvl >= 0; lvl-- {
			if pq.levels[lvl].pendingLen() > 0 {
				req, receipt, err := pq.levels[lvl].dequeueNonBlocking()
				if err == nil {
					pq.metrics.IncCounter("queue_dequeue_total", 1, hermes.Label{Key: "queue", Value: pq.queueName})
					pq.metrics.SetGauge("queue_depth", float64(pq.Len(ctx)), hermes.Label{Key: "queue", Value: pq.queueName})
					return req, fmt.Sprintf("pq-%d-%s", lvl, receipt), nil
				}
			}
		}

		// Nothing ready — wait for a signal or context cancellation.
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-pq.ready:
			// An item arrived; loop back and drain from the top again.
		}
	}
}

// Ack resolves the encoded receipt back to the originating sub-queue.
// The receipt format produced by Dequeue is "pq-<level>-<inner-receipt>".
func (pq *PriorityQueue) Ack(ctx context.Context, receipt string) error {
	lvl, inner, err := parseReceipt(receipt)
	if err != nil {
		return err
	}
	return pq.levels[lvl].Ack(ctx, inner)
}

// Nack re-queues the item at its original priority tier.
func (pq *PriorityQueue) Nack(ctx context.Context, receipt string, reason string) error {
	lvl, inner, err := parseReceipt(receipt)
	if err != nil {
		pq.metrics.IncCounter("queue_nack_errors_total", 1, hermes.Label{Key: "queue", Value: pq.queueName})
		return err
	}
	if err := pq.levels[lvl].Nack(ctx, inner, reason); err != nil {
		pq.metrics.IncCounter("queue_nack_errors_total", 1, hermes.Label{Key: "queue", Value: pq.queueName})
		return err
	}
	// Re-signal so Dequeue wakes up for the re-enqueued item.
	select {
	case pq.ready <- lvl:
	default:
	}
	pq.metrics.IncCounter("queue_nack_total", 1, hermes.Label{Key: "queue", Value: pq.queueName})
	return nil
}

// Len returns the total depth across all priority tiers.
func (pq *PriorityQueue) Len(ctx context.Context) int {
	total := 0
	for _, q := range pq.levels {
		total += q.Len(ctx)
	}
	return total
}

// parseReceipt decodes the "pq-<level>-<inner>" format back into components.
func parseReceipt(receipt string) (int, string, error) {
	var lvl int
	var inner string
	_, err := fmt.Sscanf(receipt, "pq-%d-", &lvl)
	if err != nil {
		return 0, "", fmt.Errorf("acheron: malformed priority receipt %q: %w", receipt, err)
	}
	// Strip the "pq-N-" prefix to get the inner receipt.
	prefix := fmt.Sprintf("pq-%d-", lvl)
	if len(receipt) <= len(prefix) {
		return 0, "", fmt.Errorf("acheron: malformed priority receipt %q: inner part missing", receipt)
	}
	inner = receipt[len(prefix):]
	if lvl < 0 || lvl >= NumPriorityLevels {
		return 0, "", fmt.Errorf("acheron: priority level %d out of range in receipt %q", lvl, receipt)
	}
	return lvl, inner, nil
}
