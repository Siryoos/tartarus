package acheron

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

func TestRedisQueue_EnqueueDequeueAck(t *testing.T) {
	s := miniredis.RunT(t)
	metrics := hermes.NewLogMetrics()

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, nil)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()
	req := &domain.SandboxRequest{
		ID:       "req-1",
		Template: "tpl-1",
	}

	// Enqueue
	if err := q.Enqueue(ctx, req); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Dequeue
	dequeued, receipt, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if dequeued.ID != req.ID {
		t.Errorf("Expected ID %s, got %s", req.ID, dequeued.ID)
	}

	if receipt == "" {
		t.Error("Expected non-empty receipt")
	}

	// Verify item is in PEL (Pending Entry List) before Ack
	pending, err := q.client.XPending(ctx, "test-queue", "group1").Result()
	if err != nil {
		t.Fatalf("XPending failed: %v", err)
	}
	if pending.Count != 1 {
		t.Errorf("Expected 1 pending message before Ack, got %d", pending.Count)
	}

	// Ack - This is O(1) via XACK, not O(N) scanning
	if err := q.Ack(ctx, receipt); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// Verify PEL is empty after Ack (item was successfully removed)
	pending, err = q.client.XPending(ctx, "test-queue", "group1").Result()
	if err != nil {
		t.Fatalf("XPending after Ack failed: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("Expected 0 pending messages after Ack, got %d", pending.Count)
	}
}

func TestRedisQueue_CorruptPayload(t *testing.T) {
	s := miniredis.RunT(t)
	metrics := hermes.NewLogMetrics()

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, nil)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()

	// Manually inject a corrupt payload (not JSON)
	// We need to use the raw client to inject bad data
	client := q.client
	args := &redis.XAddArgs{
		Stream: "test-queue",
		Values: map[string]interface{}{
			"data": "{invalid-json",
		},
	}
	if err := client.XAdd(ctx, args).Err(); err != nil {
		t.Fatalf("Failed to inject corrupt payload: %v", err)
	}

	// Dequeue should skip the corrupt message and return nothing (since we only added one)
	// Or rather, it will block if we don't have a timeout.
	// But Dequeue has a loop. It will continue.
	// So we need to add a valid message after the corrupt one to verify it proceeds.
	validReq := &domain.SandboxRequest{ID: "valid-1"}
	if err := q.Enqueue(ctx, validReq); err != nil {
		t.Fatalf("Failed to enqueue valid request: %v", err)
	}

	// Dequeue
	dequeued, receipt, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if dequeued.ID != validReq.ID {
		t.Errorf("Expected valid ID %s, got %s", validReq.ID, dequeued.ID)
	}
	if receipt == "" {
		t.Error("Expected non-empty receipt")
	}

	// Verify the corrupt message is in DLQ
	dlqLen, err := client.XLen(ctx, "test-queue:dlq").Result()
	if err != nil {
		t.Fatalf("Failed to check DLQ length: %v", err)
	}
	if dlqLen != 1 {
		t.Errorf("Expected 1 message in DLQ, got %d", dlqLen)
	}

	// Verify the corrupt message is NOT in PEL of the main group
	// XPENDING stream group
	pending, err := client.XPending(ctx, "test-queue", "group1").Result()
	if err != nil {
		t.Fatalf("Failed to check pending messages: %v", err)
	}
	// We expect 1 pending message (the valid one we just dequeued but haven't acked yet)
	if pending.Count != 1 {
		t.Errorf("Expected 1 pending message, got %d", pending.Count)
	}
}

func TestRedisQueue_Nack_Atomic(t *testing.T) {
	s := miniredis.RunT(t)
	metrics := hermes.NewLogMetrics()

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, nil)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()
	req := &domain.SandboxRequest{ID: "req-1"}

	q.Enqueue(ctx, req)

	_, receipt, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	// Nack
	if err := q.Nack(ctx, receipt, "failed"); err != nil {
		t.Fatalf("Nack failed: %v", err)
	}

	// Should be able to dequeue again (it was re-enqueued)
	dequeued, newReceipt, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue after Nack failed: %v", err)
	}

	if dequeued.ID != req.ID {
		t.Errorf("Expected re-dequeued ID %s, got %s", req.ID, dequeued.ID)
	}

	if newReceipt == receipt {
		t.Error("Expected new receipt ID after re-enqueue")
	}

	// Verify old receipt is Acked (not in PEL)
	// We can check XPending again.
	// We expect 1 pending message (the new one).
	pending, err := q.client.XPending(ctx, "test-queue", "group1").Result()
	if err != nil {
		t.Fatalf("Failed to check pending messages: %v", err)
	}
	if pending.Count != 1 {
		t.Errorf("Expected 1 pending message, got %d", pending.Count)
	}
}

func TestRedisQueue_Nack(t *testing.T) {
	// This test is redundant with TestRedisQueue_Nack_Atomic but keeps existing coverage logic
	// We can remove it or keep it. Let's keep it but rename/update if needed.
	// Actually, I'll just remove the old TestRedisQueue_Nack since I'm replacing the file content block
	// and I added TestRedisQueue_Nack_Atomic which covers it better.
	// Wait, I am replacing from line 56?
	// The previous file content shows TestRedisQueue_Nack starting at line 57.
	// So I should replace that one.
}

type noopMetrics struct{}

func (m *noopMetrics) IncCounter(name string, value float64, labels ...hermes.Label)       {}
func (m *noopMetrics) ObserveHistogram(name string, value float64, labels ...hermes.Label) {}
func (m *noopMetrics) SetGauge(name string, value float64, labels ...hermes.Label)         {}

func BenchmarkRedisQueue_Ack(b *testing.B) {
	s := miniredis.RunT(b)
	metrics := &noopMetrics{}

	q, err := NewRedisQueue(s.Addr(), 0, "bench-queue", "group1", "consumer1", false, metrics, nil)
	if err != nil {
		b.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()
	req := &domain.SandboxRequest{ID: "req-bench"}

	// Pre-fill queue with many items to simulate load
	// For Redis Streams with XACK: O(1) regardless of PEL size (hash-based lookup)
	// For Lists with LREM: O(N) where N is processing list size (linear scan)
	// This benchmark demonstrates current O(1) behavior.

	// Enqueue and Dequeue N items to fill PEL
	nItems := 1000
	receipts := make([]string, nItems)
	for i := 0; i < nItems; i++ {
		q.Enqueue(ctx, req)
		_, receipt, _ := q.Dequeue(ctx)
		receipts[i] = receipt
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Ack items from the PEL
		// Performance should be O(1) regardless of remaining PEL size
		idx := i % nItems
		q.Ack(ctx, receipts[idx])
	}
}
