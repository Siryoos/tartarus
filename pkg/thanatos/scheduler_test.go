package thanatos

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

func TestDeferredScheduler_Schedule(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		PolicyResolver: NewStaticPolicyResolver(nil),
		Metrics:        metrics,
		Logger:         logger,
	})

	scheduler := NewDeferredScheduler(controller, nil)

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "schedule-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Schedule termination with no delay
	resp, err := scheduler.Schedule(ctx, &DeferredTerminationRequest{
		SandboxID: req.ID,
		Delay:     0,
		Reason:    "test",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.TerminationID)
	require.Equal(t, req.ID, resp.SandboxID)
	require.Equal(t, StatusPending, resp.Status)

	// Wait for termination to complete
	time.Sleep(100 * time.Millisecond)

	// Check status
	status, err := scheduler.Get(resp.TerminationID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, status.Status)
}

func TestDeferredScheduler_ScheduleDeferred(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		PolicyResolver: NewStaticPolicyResolver(nil),
		Metrics:        metrics,
		Logger:         logger,
	})

	scheduler := NewDeferredScheduler(controller, nil)

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "deferred-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Schedule termination with 200ms delay
	resp, err := scheduler.Schedule(ctx, &DeferredTerminationRequest{
		SandboxID: req.ID,
		Delay:     200 * time.Millisecond,
		Reason:    "deferred_test",
	})
	require.NoError(t, err)
	require.Equal(t, StatusPending, resp.Status)

	// Check immediately - should still be pending
	status, err := scheduler.Get(resp.TerminationID)
	require.NoError(t, err)
	require.Equal(t, StatusPending, status.Status)

	// Wait for delay to pass
	time.Sleep(300 * time.Millisecond)

	// Should now be completed
	status, err = scheduler.Get(resp.TerminationID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, status.Status)
}

func TestDeferredScheduler_Cancel(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		PolicyResolver: NewStaticPolicyResolver(nil),
		Metrics:        metrics,
		Logger:         logger,
	})

	scheduler := NewDeferredScheduler(controller, nil)

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "cancel-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Schedule termination with long delay
	resp, err := scheduler.Schedule(ctx, &DeferredTerminationRequest{
		SandboxID: req.ID,
		Delay:     5 * time.Second,
		Reason:    "cancel_test",
	})
	require.NoError(t, err)

	// Cancel immediately
	err = scheduler.Cancel(resp.TerminationID)
	require.NoError(t, err)

	// Check status
	status, err := scheduler.Get(resp.TerminationID)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, status.Status)

	// Try to cancel again - should error
	err = scheduler.Cancel(resp.TerminationID)
	require.Error(t, err)
	require.Equal(t, ErrTerminationAlreadyCancelled, err)
}

func TestDeferredScheduler_GetBySandbox(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		PolicyResolver: NewStaticPolicyResolver(nil),
		Metrics:        metrics,
		Logger:         logger,
	})

	scheduler := NewDeferredScheduler(controller, nil)

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "bysandbox-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Schedule termination
	_, err = scheduler.Schedule(ctx, &DeferredTerminationRequest{
		SandboxID: req.ID,
		Delay:     1 * time.Second,
		Reason:    "test",
	})
	require.NoError(t, err)

	// Lookup by sandbox ID
	status, err := scheduler.GetBySandbox(req.ID)
	require.NoError(t, err)
	require.Equal(t, req.ID, status.SandboxID)

	// Non-existent sandbox
	_, err = scheduler.GetBySandbox("non-existent")
	require.Error(t, err)
	require.Equal(t, ErrTerminationNotFound, err)
}

func TestDeferredScheduler_ListCheckpoints(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	store, err := erebus.NewLocalStore(t.TempDir())
	require.NoError(t, err)

	hypnosManager := hypnos.NewManager(runtime, store, t.TempDir())

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		Hypnos:         hypnosManager,
		PolicyResolver: NewStaticPolicyResolver(nil),
		Metrics:        metrics,
		Logger:         logger,
	})

	scheduler := NewDeferredScheduler(controller, hypnosManager)

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "checkpoint-list-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err = runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Initially no checkpoints
	checkpoints, err := scheduler.ListCheckpoints(ctx, req.ID)
	require.NoError(t, err)
	require.Empty(t, checkpoints)

	// Create a checkpoint via Hypnos
	_, err = hypnosManager.Sleep(ctx, req.ID, &hypnos.SleepOptions{GracefulShutdown: true})
	require.NoError(t, err)

	// Now should have a checkpoint
	checkpoints, err = scheduler.ListCheckpoints(ctx, req.ID)
	require.NoError(t, err)
	require.Len(t, checkpoints, 1)
	require.Equal(t, req.ID, checkpoints[0].SandboxID)
}

func TestDeferredScheduler_Resume(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	store, err := erebus.NewLocalStore(t.TempDir())
	require.NoError(t, err)

	hypnosManager := hypnos.NewManager(runtime, store, t.TempDir())

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		Hypnos:         hypnosManager,
		PolicyResolver: NewStaticPolicyResolver(nil),
		Metrics:        metrics,
		Logger:         logger,
	})

	scheduler := NewDeferredScheduler(controller, hypnosManager)

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "resume-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err = runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Create a checkpoint
	record, err := hypnosManager.Sleep(ctx, req.ID, &hypnos.SleepOptions{GracefulShutdown: true})
	require.NoError(t, err)

	// Resume from checkpoint
	resp, err := scheduler.Resume(ctx, &ResumeRequest{
		CheckpointID: record.SnapshotKey,
	})
	require.NoError(t, err)
	require.Equal(t, req.ID, resp.SandboxID)
	require.Equal(t, "resumed", resp.Status)
	require.Equal(t, record.SnapshotKey, resp.ResumedFrom)
}

func TestDeferredScheduler_ResumeNotFound(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	store, err := erebus.NewLocalStore(t.TempDir())
	require.NoError(t, err)

	hypnosManager := hypnos.NewManager(runtime, store, t.TempDir())

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		Hypnos:         hypnosManager,
		PolicyResolver: NewStaticPolicyResolver(nil),
		Metrics:        metrics,
		Logger:         logger,
	})

	scheduler := NewDeferredScheduler(controller, hypnosManager)

	// Try to resume non-existent checkpoint
	_, err = scheduler.Resume(ctx, &ResumeRequest{
		CheckpointID: "non-existent-checkpoint",
	})
	require.Error(t, err)
	require.Equal(t, ErrCheckpointNotFound, err)
}

func TestDeferredScheduler_Cleanup(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		PolicyResolver: NewStaticPolicyResolver(nil),
		Metrics:        metrics,
		Logger:         logger,
	})

	scheduler := NewDeferredScheduler(controller, nil)

	// Launch and terminate multiple sandboxes
	for i := 0; i < 5; i++ {
		sandboxID := domain.SandboxID("cleanup-test-" + string(rune('0'+i)))
		req := &domain.SandboxRequest{
			ID:       sandboxID,
			Template: "test-template",
		}
		cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
		_, err := runtime.Launch(ctx, req, cfg)
		require.NoError(t, err)

		_, err = scheduler.Schedule(ctx, &DeferredTerminationRequest{
			SandboxID: sandboxID,
			Delay:     0,
		})
		require.NoError(t, err)
	}

	// Wait for them to complete
	time.Sleep(200 * time.Millisecond)

	// Cleanup - with 0 maxAge everything older than now should be cleaned
	removed := scheduler.Cleanup(0)
	require.Equal(t, 5, removed)
}

func TestMapStringToTerminationReason(t *testing.T) {
	tests := []struct {
		input    string
		expected TerminationReason
	}{
		{"user_request", ReasonUserRequest},
		{"user", ReasonUserRequest},
		{"policy_breach", ReasonPolicyBreach},
		{"policy", ReasonPolicyBreach},
		{"resource_limit", ReasonResourceLimit},
		{"resources", ReasonResourceLimit},
		{"time_limit", ReasonTimeLimit},
		{"timeout", ReasonTimeLimit},
		{"system_shutdown", ReasonSystemShutdown},
		{"shutdown", ReasonSystemShutdown},
		{"unknown", ReasonUserRequest}, // default
		{"", ReasonUserRequest},        // empty defaults
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapStringToTerminationReason(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
