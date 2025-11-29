package acheron

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// TestRedisQueue_DLQ_Metadata verifies that DLQ entries include error metadata
func TestRedisQueue_DLQ_Metadata(t *testing.T) {
	s := miniredis.RunT(t)
	metrics := hermes.NewLogMetrics()

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, nil)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()

	// Inject corrupt JSON payload directly via Redis client
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

	// Add valid message to trigger dequeue
	validReq := &domain.SandboxRequest{ID: "valid-1"}
	if err := q.Enqueue(ctx, validReq); err != nil {
		t.Fatalf("Failed to enqueue valid request: %v", err)
	}

	// Dequeue should process corrupt message to DLQ, then return valid one
	dequeued, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if dequeued.ID != validReq.ID {
		t.Errorf("Expected valid ID %s, got %s", validReq.ID, dequeued.ID)
	}

	// Verify DLQ contains the corrupt message with metadata
	dlqLen, err := client.XLen(ctx, "test-queue:dlq").Result()
	if err != nil {
		t.Fatalf("Failed to check DLQ length: %v", err)
	}
	if dlqLen != 1 {
		t.Errorf("Expected 1 message in DLQ, got %d", dlqLen)
	}

	// Check that error metadata fields exist
	msgs, err := client.XRange(ctx, "test-queue:dlq", "-", "+").Result()
	if err != nil || len(msgs) != 1 {
		t.Fatalf("Failed to read DLQ or wrong count: err=%v, len=%d", err, len(msgs))
	}

	// Verify metadata fields are present
	dlqEntry := msgs[0]
	errorReason, hasErrorReason := dlqEntry.Values["error_reason"].(string)
	_, hasOriginalID := dlqEntry.Values["original_id"].(string)
	_, hasTimestamp := dlqEntry.Values["dlq_timestamp"].(string)
	_, hasData := dlqEntry.Values["data"].(string)

	if !hasErrorReason {
		t.Error("DLQ entry missing error_reason field")
	} else if errorReason != "json_unmarshal_error" {
		t.Errorf("Expected error_reason='json_unmarshal_error', got %s", errorReason)
	}
	if !hasOriginalID {
		t.Error("DLQ entry missing original_id field")
	}
	if !hasTimestamp {
		t.Error("DLQ entry missing dlq_timestamp field")
	}
	if !hasData {
		t.Error("DLQ entry missing data field")
	}
}

// TestRedisQueue_DLQ_InvalidType verifies handling of missing data field
func TestRedisQueue_DLQ_InvalidType(t *testing.T) {
	s := miniredis.RunT(t)
	metrics := hermes.NewLogMetrics()

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, nil)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()

	// Inject payload without "data" field (will cause type assertion failure)
	client := q.client
	args := &redis.XAddArgs{
		Stream: "test-queue",
		Values: map[string]interface{}{
			"other_field": "value",
		},
	}
	if err := client.XAdd(ctx, args).Err(); err != nil {
		t.Fatalf("Failed to inject invalid format payload: %v", err)
	}

	// Add valid message
	validReq := &domain.SandboxRequest{ID: "valid-1"}
	if err := q.Enqueue(ctx, validReq); err != nil {
		t.Fatalf("Failed to enqueue valid request: %v", err)
	}

	// Dequeue should skip invalid format and return valid message
	dequeued, _, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if dequeued.ID != validReq.ID {
		t.Errorf("Expected valid ID %s, got %s", validReq.ID, dequeued.ID)
	}

	// Verify DLQ entry exists
	dlqLen, err := client.XLen(ctx, "test-queue:dlq").Result()
	if err != nil {
		t.Fatalf("Failed to check DLQ length: %v", err)
	}
	if dlqLen != 1 {
		t.Errorf("Expected 1 message in DLQ, got %d", dlqLen)
	}

	// Check error reason
	msgs, err := client.XRange(ctx, "test-queue:dlq", "-", "+").Result()
	if err != nil {
		t.Fatalf("Failed to read DLQ: %v", err)
	}
	if len(msgs) > 0 {
		if errorReason, ok := msgs[0].Values["error_reason"].(string); ok {
			if errorReason != "invalid_payload_format" {
				t.Errorf("Expected error_reason='invalid_payload_format', got %s", errorReason)
			}
		} else {
			t.Error("DLQ entry missing error_reason field")
		}
	}
}

// TestRedisQueue_DLQ_MultiplePoisons verifies multiple corrupt messages are handled
func TestRedisQueue_DLQ_MultiplePoisons(t *testing.T) {
	s := miniredis.RunT(t)
	metrics := hermes.NewLogMetrics()

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, nil)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()
	client := q.client

	// Inject 3 corrupt payloads
	corruptPayloads := []string{"{bad-json-1", "{bad-json-2", "{{incomplete"}
	for _, payload := range corruptPayloads {
		args := &redis.XAddArgs{
			Stream: "test-queue",
			Values: map[string]interface{}{
				"data": payload,
			},
		}
		if err := client.XAdd(ctx, args).Err(); err != nil {
			t.Fatalf("Failed to inject corrupt payload: %v", err)
		}
	}

	// Add 2 valid messages
	for i := 0; i < 2; i++ {
		req := &domain.SandboxRequest{ID: domain.SandboxID(fmt.Sprintf("valid-%d", i))}
		if err := q.Enqueue(ctx, req); err != nil {
			t.Fatalf("Failed to enqueue valid request: %v", err)
		}
	}

	// Dequeue valid messages - should skip all corrupt ones
	for i := 0; i < 2; i++ {
		dequeued, _, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue %d failed: %v", i, err)
		}
		expectedID := domain.SandboxID(fmt.Sprintf("valid-%d", i))
		if dequeued.ID != expectedID {
			t.Errorf("Expected ID %s, got %s", expectedID, dequeued.ID)
		}
	}

	// Verify all 3 corrupt messages are in DLQ
	dlqLen, err := client.XLen(ctx, "test-queue:dlq").Result()
	if err != nil {
		t.Fatalf("Failed to check DLQ length: %v", err)
	}
	if dlqLen != 3 {
		t.Errorf("Expected 3 messages in DLQ, got %d", dlqLen)
	}

	// Verify all DLQ entries have error_reason
	msgs, err := client.XRange(ctx, "test-queue:dlq", "-", "+").Result()
	if err != nil {
		t.Fatalf("Failed to read DLQ: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("Expected 3 DLQ entries, got %d", len(msgs))
	}

	for idx, msg := range msgs {
		if errorReason, ok := msg.Values["error_reason"].(string); ok {
			// All should be JSON errors since they're all malformed JSON
			if errorReason != "json_unmarshal_error" {
				t.Errorf("Entry %d: expected json_unmarshal_error, got %s", idx, errorReason)
			}
		} else {
			t.Errorf("Entry %d missing error_reason", idx)
		}
	}
}

// TestRedisQueue_Nack_Metrics verifies Nack metric emission
func TestRedisQueue_Nack_Metrics(t *testing.T) {
	s := miniredis.RunT(t)

	// Track metric calls
	type metricsCall struct {
		name  string
		value float64
	}
	var calls []metricsCall
	mockMetrics := &mockMetricsRecorder{
		onIncCounter: func(name string, value float64, labels ...hermes.Label) {
			calls = append(calls, metricsCall{name: name, value: value})
		},
	}

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, mockMetrics, nil)
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

	// Clear calls from enqueue/dequeue
	calls = nil

	// Nack should emit metric
	if err := q.Nack(ctx, receipt, "test failure"); err != nil {
		t.Fatalf("Nack failed: %v", err)
	}

	// Verify queue_nack_total was incremented
	found := false
	for _, call := range calls {
		if call.name == "queue_nack_total" && call.value == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected queue_nack_total metric, got calls: %+v", calls)
	}
}

// mockMetricsRecorder is a test helper for tracking metric calls
type mockMetricsRecorder struct {
	onIncCounter       func(string, float64, ...hermes.Label)
	onObserveHistogram func(string, float64, ...hermes.Label)
	onSetGauge         func(string, float64, ...hermes.Label)
}

func (m *mockMetricsRecorder) IncCounter(name string, value float64, labels ...hermes.Label) {
	if m.onIncCounter != nil {
		m.onIncCounter(name, value, labels...)
	}
}

func (m *mockMetricsRecorder) ObserveHistogram(name string, value float64, labels ...hermes.Label) {
	if m.onObserveHistogram != nil {
		m.onObserveHistogram(name, value, labels...)
	}
}

func (m *mockMetricsRecorder) SetGauge(name string, value float64, labels ...hermes.Label) {
	if m.onSetGauge != nil {
		m.onSetGauge(name, value, labels...)
	}
}
