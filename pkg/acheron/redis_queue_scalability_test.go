package acheron

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// TestRedisQueue_Ack_ScalabilityRegression verifies that Ack performance
// remains O(1) regardless of PEL (Pending Entry List) size.
// This test prevents regression to O(N) scanning behavior.
func TestRedisQueue_Ack_ScalabilityRegression(t *testing.T) {
	s := miniredis.RunT(t)
	metrics := &noopMetrics{}

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()

	testCases := []struct {
		name    string
		pelSize int
	}{
		{"PEL_10", 10},
		{"PEL_100", 100},
		{"PEL_1000", 1000},
		{"PEL_10000", 10000},
	}

	// Store timing results to verify O(1) behavior
	timings := make(map[int]time.Duration)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear queue between test cases
			s.FlushDB()
			q.client.XGroupCreateMkStream(ctx, "test-queue", "group1", "0")

			// Fill PEL with items
			receipts := make([]string, tc.pelSize)
			req := &domain.SandboxRequest{ID: "test-req"}

			for i := 0; i < tc.pelSize; i++ {
				if err := q.Enqueue(ctx, req); err != nil {
					t.Fatalf("Enqueue failed: %v", err)
				}
				_, receipt, err := q.Dequeue(ctx)
				if err != nil {
					t.Fatalf("Dequeue failed: %v", err)
				}
				receipts[i] = receipt
			}

			// Verify PEL size
			pending, err := q.client.XPending(ctx, "test-queue", "group1").Result()
			if err != nil {
				t.Fatalf("XPending failed: %v", err)
			}
			if pending.Count != int64(tc.pelSize) {
				t.Fatalf("Expected PEL size %d, got %d", tc.pelSize, pending.Count)
			}

			// Measure Ack time for middle item (worst case for O(N) scan)
			targetIdx := tc.pelSize / 2
			start := time.Now()
			if err := q.Ack(ctx, receipts[targetIdx]); err != nil {
				t.Fatalf("Ack failed: %v", err)
			}
			elapsed := time.Since(start)
			timings[tc.pelSize] = elapsed

			t.Logf("PEL size %d: Ack time %v", tc.pelSize, elapsed)

			// Verify item was actually removed from PEL
			pending, err = q.client.XPending(ctx, "test-queue", "group1").Result()
			if err != nil {
				t.Fatalf("XPending after Ack failed: %v", err)
			}
			if pending.Count != int64(tc.pelSize-1) {
				t.Errorf("Expected PEL size %d after Ack, got %d", tc.pelSize-1, pending.Count)
			}
		})
	}

	// Verify O(1) behavior: time should not scale linearly with PEL size
	// Allow up to 3x variance (generous margin for test environment noise)
	baseTime := timings[10]
	for pelSize, timing := range timings {
		if pelSize == 10 {
			continue
		}

		// For O(1), we expect timing to be roughly constant
		// For O(N), timing would scale with pelSize/10
		// Allow 3x variance to account for Redis overhead and test noise
		maxAllowed := baseTime * 3

		if timing > maxAllowed {
			t.Errorf("Ack performance degradation detected: PEL=%d took %v (base=%v, max allowed=%v). This suggests O(N) scanning behavior.",
				pelSize, timing, baseTime, maxAllowed)
		}
	}

	t.Logf("Performance profile: %v", timings)
}

// TestRedisQueue_Ack_VerifiesPELDecrement ensures that Ack actually
// removes items from the Pending Entry List.
func TestRedisQueue_Ack_VerifiesPELDecrement(t *testing.T) {
	s := miniredis.RunT(t)
	metrics := hermes.NewLogMetrics()

	q, err := NewRedisQueue(s.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	ctx := context.Background()

	// Enqueue and dequeue multiple items
	numItems := 5
	receipts := make([]string, numItems)
	for i := 0; i < numItems; i++ {
		req := &domain.SandboxRequest{ID: domain.SandboxID("req-" + string(rune('0'+i)))}
		if err := q.Enqueue(ctx, req); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
		_, receipt, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue failed: %v", err)
		}
		receipts[i] = receipt
	}

	// Verify initial PEL size
	pending, err := q.client.XPending(ctx, "test-queue", "group1").Result()
	if err != nil {
		t.Fatalf("XPending failed: %v", err)
	}
	if pending.Count != int64(numItems) {
		t.Fatalf("Expected initial PEL size %d, got %d", numItems, pending.Count)
	}

	// Ack items one by one and verify PEL decrements
	for i, receipt := range receipts {
		if err := q.Ack(ctx, receipt); err != nil {
			t.Fatalf("Ack %d failed: %v", i, err)
		}

		pending, err := q.client.XPending(ctx, "test-queue", "group1").Result()
		if err != nil {
			t.Fatalf("XPending after Ack %d failed: %v", i, err)
		}

		expectedCount := int64(numItems - i - 1)
		if pending.Count != expectedCount {
			t.Errorf("After Ack %d: expected PEL size %d, got %d", i, expectedCount, pending.Count)
		}
	}

	// Verify PEL is now empty
	pending, err = q.client.XPending(ctx, "test-queue", "group1").Result()
	if err != nil {
		t.Fatalf("Final XPending failed: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("Expected empty PEL, got %d items", pending.Count)
	}
}

// BenchmarkRedisQueue_Ack_VariablePEL benchmarks Ack performance with different
// Pending Entry List sizes to demonstrate O(1) behavior.
func BenchmarkRedisQueue_Ack_VariablePEL(b *testing.B) {
	pelSizes := []int{10, 100, 1000, 10000}

	for _, pelSize := range pelSizes {
		b.Run(formatPELSize(pelSize), func(b *testing.B) {
			s := miniredis.RunT(b)
			metrics := &noopMetrics{}

			q, err := NewRedisQueue(s.Addr(), 0, "bench-queue", "group1", "consumer1", false, metrics)
			if err != nil {
				b.Fatalf("Failed to create queue: %v", err)
			}

			ctx := context.Background()
			req := &domain.SandboxRequest{ID: "req-bench"}

			// Pre-fill PEL
			receipts := make([]string, pelSize)
			for i := 0; i < pelSize; i++ {
				q.Enqueue(ctx, req)
				_, receipt, _ := q.Dequeue(ctx)
				receipts[i] = receipt
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % pelSize
				q.Ack(ctx, receipts[idx])
			}
		})
	}
}

func formatPELSize(size int) string {
	switch size {
	case 10:
		return "PEL_10"
	case 100:
		return "PEL_100"
	case 1000:
		return "PEL_1000"
	case 10000:
		return "PEL_10000"
	default:
		return "PEL_Unknown"
	}
}
