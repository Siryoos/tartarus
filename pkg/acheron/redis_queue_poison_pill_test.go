package acheron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// Test that poison pills are written to Cocytus sink
func TestRedisQueue_PoisonPill_CocytusIntegration(t *testing.T) {
	s := miniredis.RunT(t)

	metrics := hermes.NewLogMetrics()

	// Mock Cocytus sink to track writes
	sink := &mockCocytusSink{}

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, sink)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()
	client := q.client

	// Inject invalid JSON payload directly
	args := &redis.XAddArgs{
		Stream: "test-queue",
		Values: map[string]interface{}{
			"data": "{invalid-json",
		},
	}
	if err := client.XAdd(ctx, args).Err(); err != nil {
		t.Fatalf("Failed to inject corrupt payload: %v", err)
	}

	// Add a valid message so dequeue returns
	validReq := &domain.SandboxRequest{ID: "valid-1"}
	if err := q.Enqueue(ctx, validReq); err != nil {
		t.Fatalf("Failed to enqueue valid request: %v", err)
	}

	// Dequeue should skip poison pill and return valid message
	req, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if req.ID != validReq.ID {
		t.Fatal("Expected valid request after skipping poison pill")
	}

	// Wait for async Cocytus write
	time.Sleep(100 * time.Millisecond)

	// Verify Cocytus sink was called
	if sink.written == nil {
		t.Fatal("Expected Cocytus sink to be called")
	}

	if sink.written.Reason != "poison_pill: json_unmarshal_error" {
		t.Errorf("Expected reason 'poison_pill: json_unmarshal_error', got %s", sink.written.Reason)
	}

	if len(sink.written.Payload) == 0 {
		t.Error("Expected payload to be captured")
	}

	// Verify DLQ contains the message
	dlqLen, err := client.XLen(ctx, "test-queue:dlq").Result()
	if err != nil {
		t.Fatalf("Failed to check DLQ length: %v", err)
	}
	if dlqLen != 1 {
		t.Errorf("Expected DLQ length 1, got %d", dlqLen)
	}
}

// Test that Cocytus write failures don't prevent DLQ move
func TestRedisQueue_PoisonPill_CocytusFailure(t *testing.T) {
	s := miniredis.RunT(t)

	metrics := hermes.NewLogMetrics()

	// Mock Cocytus sink that fails
	sink := &mockCocytusSink{shouldFail: true}

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, sink)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()
	client := q.client

	// Inject invalid JSON payload
	args := &redis.XAddArgs{
		Stream: "test-queue",
		Values: map[string]interface{}{
			"data": "{invalid-json",
		},
	}
	if err := client.XAdd(ctx, args).Err(); err != nil {
		t.Fatalf("Failed to inject corrupt payload: %v", err)
	}

	// Add a valid message
	validReq := &domain.SandboxRequest{ID: "valid-1"}
	if err := q.Enqueue(ctx, validReq); err != nil {
		t.Fatalf("Failed to enqueue valid request: %v", err)
	}

	// Dequeue should still work even if Cocytus write fails
	req, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if req.ID != validReq.ID {
		t.Fatal("Expected valid request after skipping poison pill")
	}

	// Wait for async Cocytus write attempt
	time.Sleep(100 * time.Millisecond)

	// Verify DLQ still contains the message (best-effort Cocytus write)
	dlqLen, err := client.XLen(ctx, "test-queue:dlq").Result()
	if err != nil {
		t.Fatalf("Failed to check DLQ length: %v", err)
	}
	if dlqLen != 1 {
		t.Errorf("Expected DLQ length 1, got %d", dlqLen)
	}
}

// Test that nil sink is handled gracefully
func TestRedisQueue_PoisonPill_NilSink(t *testing.T) {
	s := miniredis.RunT(t)

	metrics := hermes.NewLogMetrics()

	// Create queue with nil sink (backward compatibility)
	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, nil)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()
	client := q.client

	// Inject invalid JSON payload
	args := &redis.XAddArgs{
		Stream: "test-queue",
		Values: map[string]interface{}{
			"data": "{invalid-json",
		},
	}
	if err := client.XAdd(ctx, args).Err(); err != nil {
		t.Fatalf("Failed to inject corrupt payload: %v", err)
	}

	// Add a valid message
	validReq := &domain.SandboxRequest{ID: "valid-1"}
	if err := q.Enqueue(ctx, validReq); err != nil {
		t.Fatalf("Failed to enqueue valid request: %v", err)
	}

	// Dequeue should still work with nil sink
	req, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if req.ID != validReq.ID {
		t.Fatal("Expected valid request after skipping poison pill")
	}

	// Verify DLQ contains the message
	dlqLen, err := client.XLen(ctx, "test-queue:dlq").Result()
	if err != nil {
		t.Fatalf("Failed to check DLQ length: %v", err)
	}
	if dlqLen != 1 {
		t.Errorf("Expected DLQ length 1, got %d", dlqLen)
	}
}

// Test invalid payload format (not a string)
func TestRedisQueue_PoisonPill_InvalidFormat(t *testing.T) {
	s := miniredis.RunT(t)

	metrics := hermes.NewLogMetrics()
	sink := &mockCocytusSink{}

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, sink)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()
	client := q.client

	// Inject a message without "data" field
	args := &redis.XAddArgs{
		Stream: "test-queue",
		Values: map[string]interface{}{
			"other": "value",
		},
	}
	if err := client.XAdd(ctx, args).Err(); err != nil {
		t.Fatalf("Failed to inject invalid format payload: %v", err)
	}

	// Add a valid message
	validReq := &domain.SandboxRequest{ID: "valid-1"}
	if err := q.Enqueue(ctx, validReq); err != nil {
		t.Fatalf("Failed to enqueue valid request: %v", err)
	}

	// Dequeue should handle it
	req, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if req.ID != validReq.ID {
		t.Fatal("Expected valid request after skipping invalid format")
	}

	// Wait for async Cocytus write
	time.Sleep(100 * time.Millisecond)

	// Verify Cocytus sink was called with invalid_payload_format reason
	if sink.written == nil {
		t.Fatal("Expected Cocytus sink to be called")
	}

	if sink.written.Reason != "poison_pill: invalid_payload_format" {
		t.Errorf("Expected reason 'poison_pill: invalid_payload_format', got %s", sink.written.Reason)
	}
}

// Mock Cocytus sink for testing
type mockCocytusSink struct {
	written    *cocytus.Record
	shouldFail bool
}

func (m *mockCocytusSink) Write(ctx context.Context, rec *cocytus.Record) error {
	m.written = rec
	if m.shouldFail {
		return errors.New("cocytus write failed")
	}
	return nil
}
