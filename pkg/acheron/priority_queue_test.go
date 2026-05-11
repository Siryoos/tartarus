package acheron

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// --- helpers ---------------------------------------------------------------

func newTestPQ(t *testing.T) *PriorityQueue {
	t.Helper()
	return NewPriorityQueue(hermes.NewNoopMetrics(), "test-pq")
}

// --- PriorityQueue tests ----------------------------------------------------

// TestPriorityQueue_HighBeforeLow verifies that a high-priority request
// enqueued after a low-priority request is still dequeued first.
func TestPriorityQueue_HighBeforeLow(t *testing.T) {
	pq := newTestPQ(t)
	ctx := context.Background()

	low := &domain.SandboxRequest{ID: "low", Priority: domain.PriorityLow}
	high := &domain.SandboxRequest{ID: "high", Priority: domain.PriorityHigh}

	if err := pq.Enqueue(ctx, low); err != nil {
		t.Fatalf("Enqueue low: %v", err)
	}
	if err := pq.Enqueue(ctx, high); err != nil {
		t.Fatalf("Enqueue high: %v", err)
	}

	first, receipt1, err := pq.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue first: %v", err)
	}
	if first.ID != "high" {
		t.Errorf("Expected 'high' first, got %q", first.ID)
	}

	second, receipt2, err := pq.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue second: %v", err)
	}
	if second.ID != "low" {
		t.Errorf("Expected 'low' second, got %q", second.ID)
	}

	// Ack both to clear processing maps.
	pq.Ack(ctx, receipt1)
	pq.Ack(ctx, receipt2)
}

// TestPriorityQueue_NormalDefaultPriority verifies that a request with zero
// Priority (omitempty) is treated as PriorityNormal and ordered correctly.
func TestPriorityQueue_NormalDefaultPriority(t *testing.T) {
	pq := newTestPQ(t)
	ctx := context.Background()

	// Zero-value priority → PriorityLow (int 0); send one of each.
	low := &domain.SandboxRequest{ID: "zero", Priority: 0}     // == PriorityLow
	normal := &domain.SandboxRequest{ID: "normal", Priority: domain.PriorityNormal}

	pq.Enqueue(ctx, low)
	pq.Enqueue(ctx, normal)

	first, r1, _ := pq.Dequeue(ctx)
	if first.ID != "normal" {
		t.Errorf("Expected 'normal' first, got %q", first.ID)
	}
	second, r2, _ := pq.Dequeue(ctx)
	if second.ID != "zero" {
		t.Errorf("Expected 'zero' second, got %q", second.ID)
	}
	pq.Ack(ctx, r1)
	pq.Ack(ctx, r2)
}

// TestPriorityQueue_ContextCancellation verifies that Dequeue returns promptly
// when the context is cancelled while blocking on an empty queue.
func TestPriorityQueue_ContextCancellation(t *testing.T) {
	pq := newTestPQ(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_, _, err := pq.Dequeue(ctx)
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Dequeue did not respect context cancellation within 2s")
	}
}

// TestPriorityQueue_Nack verifies that a Nacked message can be dequeued again.
func TestPriorityQueue_Nack(t *testing.T) {
	pq := newTestPQ(t)
	ctx := context.Background()

	req := &domain.SandboxRequest{ID: "req-1", Priority: domain.PriorityNormal}
	pq.Enqueue(ctx, req)

	_, receipt, err := pq.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}

	if err := pq.Nack(ctx, receipt, "transient failure"); err != nil {
		t.Fatalf("Nack: %v", err)
	}

	// Should be re-dequeue-able.
	requeued, newReceipt, err := pq.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue after Nack: %v", err)
	}
	if requeued.ID != req.ID {
		t.Errorf("Expected %q, got %q", req.ID, requeued.ID)
	}
	if newReceipt == receipt {
		t.Error("Receipt should be different after Nack re-enqueue")
	}
	pq.Ack(ctx, newReceipt)
}

// TestPriorityQueue_Len verifies that Len sums across all tiers.
func TestPriorityQueue_Len(t *testing.T) {
	pq := newTestPQ(t)
	ctx := context.Background()

	pq.Enqueue(ctx, &domain.SandboxRequest{ID: "a", Priority: domain.PriorityLow})
	pq.Enqueue(ctx, &domain.SandboxRequest{ID: "b", Priority: domain.PriorityNormal})
	pq.Enqueue(ctx, &domain.SandboxRequest{ID: "c", Priority: domain.PriorityHigh})

	if got := pq.Len(ctx); got != 3 {
		t.Errorf("Expected Len 3, got %d", got)
	}
}

// TestPriorityQueue_AckUnknownReceipt verifies that Ack-ing an unknown receipt
// does not crash (same silent-noop contract as MemoryQueue/RedisQueue).
func TestPriorityQueue_AckUnknownReceipt(t *testing.T) {
	pq := newTestPQ(t)
	ctx := context.Background()
	if err := pq.Ack(ctx, "pq-1-mem-receipt-9999"); err != nil {
		t.Errorf("Unexpected error Ack-ing unknown receipt: %v", err)
	}
}

// --- MemoryQueue context cancellation test ---------------------------------

// TestMemoryQueue_ContextCancellation verifies that MemoryQueue.Dequeue
// returns context.Canceled promptly when the context is cancelled while
// the queue is empty.
func TestMemoryQueue_ContextCancellation(t *testing.T) {
	q := NewMemoryQueue(hermes.NewNoopMetrics(), "test-mq")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_, _, err := q.Dequeue(ctx)
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("MemoryQueue.Dequeue did not respect context cancellation within 2s")
	}
}

// TestMemoryQueue_ContextTimeout verifies deadline cancellation.
func TestMemoryQueue_ContextTimeout(t *testing.T) {
	q := NewMemoryQueue(hermes.NewNoopMetrics(), "test-mq")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := q.Dequeue(ctx)
	elapsed := time.Since(start)

	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Dequeue took %v, expected ≤500ms after deadline", elapsed)
	}
}

// --- Instrumented queue test -----------------------------------------------

type captureMetrics struct {
	counters map[string]float64
	gauges   map[string]float64
}

func newCaptureMetrics() *captureMetrics {
	return &captureMetrics{
		counters: make(map[string]float64),
		gauges:   make(map[string]float64),
	}
}

func (c *captureMetrics) IncCounter(name string, value float64, _ ...hermes.Label) {
	c.counters[name] += value
}
func (c *captureMetrics) ObserveHistogram(name string, value float64, _ ...hermes.Label) {}
func (c *captureMetrics) SetGauge(name string, value float64, _ ...hermes.Label) {
	c.gauges[name] = value
}

func TestInstrumentedQueue_EmitsMetrics(t *testing.T) {
	m := newCaptureMetrics()
	inner := NewMemoryQueue(hermes.NewNoopMetrics(), "")
	q := NewInstrumentedQueue(inner, m, "test-iq")

	ctx := context.Background()
	req := &domain.SandboxRequest{ID: "x"}

	q.Enqueue(ctx, req)
	if got := m.counters["queue_enqueue_total"]; got != 1 {
		t.Errorf("queue_enqueue_total: want 1, got %v", got)
	}

	_, receipt, _ := q.Dequeue(ctx)
	if got := m.counters["queue_dequeue_total"]; got != 1 {
		t.Errorf("queue_dequeue_total: want 1, got %v", got)
	}

	q.Nack(ctx, receipt, "test")
	if got := m.counters["queue_nack_total"]; got != 1 {
		t.Errorf("queue_nack_total: want 1, got %v", got)
	}
}
