package integration

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// TestPhase3DLQFlows tests end-to-end DLQ behavior for poison pill handling.
func TestPhase3DLQFlows(t *testing.T) {
	// Setup infrastructure
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	metrics := hermes.NewLogMetrics()

	t.Run("PoisonPillMovesToDLQ", func(t *testing.T) {
		// Create queue
		q, err := acheron.NewRedisQueue(mr.Addr(), 0, "test-queue", "group1", "consumer1", false, metrics, nil)
		require.NoError(t, err)

		// Inject corrupt payload directly via Redis
		client := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
			DB:   0,
		})
		defer client.Close()

		args := &redis.XAddArgs{
			Stream: "test-queue",
			Values: map[string]interface{}{
				"data": "{invalid-json",
			},
		}
		err = client.XAdd(ctx, args).Err()
		require.NoError(t, err)

		// Enqueue valid request
		validReq := &domain.SandboxRequest{ID: "valid-1"}
		err = q.Enqueue(ctx, validReq)
		require.NoError(t, err)

		// Dequeue should skip corrupt message and return valid one
		dequeued, _, err := q.Dequeue(ctx)
		require.NoError(t, err)
		assert.Equal(t, validReq.ID, dequeued.ID, "Should skip corrupt message and return valid request")

		// Verify corrupt message is in DLQ
		dlqLen, err := client.XLen(ctx, "test-queue:dlq").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), dlqLen, "Corrupt message should be in DLQ")

		// Verify DLQ metadata
		msgs, err := client.XRange(ctx, "test-queue:dlq", "-", "+").Result()
		require.NoError(t, err)
		require.Len(t, msgs, 1)

		dlqEntry := msgs[0]
		errorReason, ok := dlqEntry.Values["error_reason"].(string)
		assert.True(t, ok, "DLQ entry should have error_reason")
		assert.Equal(t, "json_unmarshal_error", errorReason)

		_, hasOriginalID := dlqEntry.Values["original_id"]
		assert.True(t, hasOriginalID, "DLQ entry should have original_id")

		_, hasTimestamp := dlqEntry.Values["dlq_timestamp"]
		assert.True(t, hasTimestamp, "DLQ entry should have dlq_timestamp")
	})

	t.Run("MultiplePoisonPillsHandled", func(t *testing.T) {
		// Create queue
		q, err := acheron.NewRedisQueue(mr.Addr(), 0, "test-queue-2", "group1", "consumer1", false, metrics, nil)
		require.NoError(t, err)

		client := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
			DB:   0,
		})
		defer client.Close()

		// Inject multiple corrupt payloads
		corruptPayloads := []string{"{bad-1", "{bad-2", "{{incomplete"}
		for _, payload := range corruptPayloads {
			args := &redis.XAddArgs{
				Stream: "test-queue-2",
				Values: map[string]interface{}{
					"data": payload,
				},
			}
			err = client.XAdd(ctx, args).Err()
			require.NoError(t, err)
		}

		// Enqueue valid requests
		validReqs := []*domain.SandboxRequest{
			{ID: "valid-1"},
			{ID: "valid-2"},
		}
		for _, req := range validReqs {
			err = q.Enqueue(ctx, req)
			require.NoError(t, err)
		}

		// Dequeue all valid requests - should skip all corrupt ones
		for i := 0; i < len(validReqs); i++ {
			dequeued, _, err := q.Dequeue(ctx)
			require.NoError(t, err)
			assert.Equal(t, validReqs[i].ID, dequeued.ID)
		}

		// Verify all corrupt messages are in DLQ
		dlqLen, err := client.XLen(ctx, "test-queue-2:dlq").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(len(corruptPayloads)), dlqLen, "All corrupt messages should be in DLQ")
	})

	t.Run("InvalidPayloadFormat", func(t *testing.T) {
		// Create queue
		q, err := acheron.NewRedisQueue(mr.Addr(), 0, "test-queue-3", "group1", "consumer1", false, metrics, nil)
		require.NoError(t, err)

		client := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
			DB:   0,
		})
		defer client.Close()

		// Inject payload without "data" field
		args := &redis.XAddArgs{
			Stream: "test-queue-3",
			Values: map[string]interface{}{
				"other_field": "value",
			},
		}
		err = client.XAdd(ctx, args).Err()
		require.NoError(t, err)

		// Enqueue valid request
		validReq := &domain.SandboxRequest{ID: "valid-1"}
		err = q.Enqueue(ctx, validReq)
		require.NoError(t, err)

		// Dequeue should skip invalid format and return valid request
		dequeued, _, err := q.Dequeue(ctx)
		require.NoError(t, err)
		assert.Equal(t, validReq.ID, dequeued.ID)

		// Verify DLQ entry has correct error reason
		msgs, err := client.XRange(ctx, "test-queue-3:dlq", "-", "+").Result()
		require.NoError(t, err)
		require.Len(t, msgs, 1)

		errorReason, ok := msgs[0].Values["error_reason"].(string)
		assert.True(t, ok)
		assert.Equal(t, "invalid_payload_format", errorReason)
	})

	t.Run("CocytusIntegrationWithDLQ", func(t *testing.T) {
		// Create queue
		q, err := acheron.NewRedisQueue(mr.Addr(), 0, "test-queue-4", "group1", "consumer1", false, metrics, nil)
		require.NoError(t, err)

		// Create Cocytus sink for dead letter tracking
		cocytusSink := cocytus.NewLogSink(slog.Default())

		client := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
			DB:   0,
		})
		defer client.Close()

		// Inject corrupt payload
		args := &redis.XAddArgs{
			Stream: "test-queue-4",
			Values: map[string]interface{}{
				"data": "{corrupt-json",
			},
		}
		_, err = client.XAdd(ctx, args).Result()
		require.NoError(t, err)

		// Simulate agent trying to dequeue and failing
		// The Acheron queue will move corrupt message to its own DLQ
		validReq := &domain.SandboxRequest{ID: "valid-1"}
		err = q.Enqueue(ctx, validReq)
		require.NoError(t, err)

		dequeued, _, err := q.Dequeue(ctx)
		require.NoError(t, err)
		assert.Equal(t, validReq.ID, dequeued.ID)

		// Verify corrupt message is in Acheron DLQ
		dlqLen, err := client.XLen(ctx, "test-queue-4:dlq").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), dlqLen)

		// Agent would send failure record to Cocytus for tracking
		record := &cocytus.Record{
			RunID:     "test-run",
			RequestID: "corrupt-request",
			Reason:    "json_unmarshal_error",
			Payload:   []byte("{corrupt-json"),
			CreatedAt: time.Now(),
		}
		err = cocytusSink.Write(ctx, record)
		require.NoError(t, err, "Cocytus should accept dead letter record")

		// Verify DLQ entry has metadata
		msgs, err := client.XRange(ctx, "test-queue-4:dlq", "-", "+").Result()
		require.NoError(t, err)
		require.Len(t, msgs, 1)

		entry := msgs[0]
		assert.Equal(t, "json_unmarshal_error", entry.Values["error_reason"])
	})

	t.Run("NackMovesBackToQueue", func(t *testing.T) {
		// Verify that valid messages that are Nack'd go back to queue, not DLQ
		q, err := acheron.NewRedisQueue(mr.Addr(), 0, "test-queue-5", "group1", "consumer1", false, metrics, nil)
		require.NoError(t, err)

		// Enqueue valid request
		req := &domain.SandboxRequest{ID: "valid-nack"}
		err = q.Enqueue(ctx, req)
		require.NoError(t, err)

		// Dequeue
		dequeued, receipt, err := q.Dequeue(ctx)
		require.NoError(t, err)
		assert.Equal(t, req.ID, dequeued.ID)

		// Nack it
		err = q.Nack(ctx, receipt, "simulated failure")
		require.NoError(t, err)

		// Should be back in queue, not DLQ
		client := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
			DB:   0,
		})
		defer client.Close()

		dlqLen, err := client.XLen(ctx, "test-queue-5:dlq").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), dlqLen, "Nack'd valid message should NOT go to DLQ")

		// Dequeue again - should get the same request
		redelivered, _, err := q.Dequeue(ctx)
		require.NoError(t, err)
		assert.Equal(t, req.ID, redelivered.ID, "Nack'd message should be redelivered")
	})
}
