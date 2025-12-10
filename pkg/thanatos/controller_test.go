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

func TestShutdownController_GracefulTermination(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	policy := &GracePolicy{
		ID:           "test-policy",
		DefaultGrace: 100 * time.Millisecond,
	}
	resolver := NewStaticPolicyResolver(policy)

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		PolicyResolver: resolver,
		Metrics:        metrics,
		Logger:         logger,
	})

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "graceful-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Terminate gracefully
	result, err := controller.RequestTermination(ctx, &TerminationRequest{
		SandboxID:  req.ID,
		TemplateID: req.Template,
		Reason:     ReasonUserRequest,
	})
	require.NoError(t, err)
	require.Equal(t, PhaseCompleted, result.Phase)
	require.Equal(t, ReasonUserRequest, result.Reason)
}

func TestShutdownController_WithCheckpoint(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	store, err := erebus.NewLocalStore(t.TempDir())
	require.NoError(t, err)
	hypnosManager := hypnos.NewManager(runtime, store, t.TempDir())

	policy := &GracePolicy{
		ID:              "checkpoint-policy",
		DefaultGrace:    100 * time.Millisecond,
		CheckpointFirst: true,
	}
	resolver := NewStaticPolicyResolver(policy)

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		Hypnos:         hypnosManager,
		PolicyResolver: resolver,
		Metrics:        metrics,
		Logger:         logger,
	})

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "checkpoint-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err = runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Terminate with checkpoint
	result, err := controller.RequestTermination(ctx, &TerminationRequest{
		SandboxID:  req.ID,
		TemplateID: req.Template,
		Reason:     ReasonUserRequest,
	})
	require.NoError(t, err)
	require.Equal(t, PhaseCheckpointed, result.Phase)
	require.NotEmpty(t, result.Checkpoint)
}

func TestShutdownController_ResolvesPolicyByReason(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	defaultPolicy := &GracePolicy{
		ID:           "default",
		DefaultGrace: 10 * time.Second,
	}
	enforcementPolicy := &GracePolicy{
		ID:           "enforcement",
		DefaultGrace: 100 * time.Millisecond,
	}

	resolver := NewStaticPolicyResolver(defaultPolicy)
	resolver.SetReasonPolicy(ReasonPolicyBreach, enforcementPolicy)

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		PolicyResolver: resolver,
		Metrics:        metrics,
		Logger:         logger,
	})

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "policy-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Terminate with policy breach reason (should use short grace)
	result, err := controller.RequestTermination(ctx, &TerminationRequest{
		SandboxID:  req.ID,
		TemplateID: req.Template,
		Reason:     ReasonPolicyBreach,
	})
	require.NoError(t, err)
	require.Equal(t, PhaseCompleted, result.Phase)
	require.Equal(t, "enforcement", result.Policy.ID)
}

func TestShutdownController_GraceTimeoutForceKill(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	// Set a delay that exceeds the grace period
	runtime.ShutdownDelay = 500 * time.Millisecond
	metrics := hermes.NewNoopMetrics()
	logger := hermes.NewSlogAdapter()

	policy := &GracePolicy{
		ID:           "short-grace",
		DefaultGrace: 50 * time.Millisecond, // Very short grace
	}
	resolver := NewStaticPolicyResolver(policy)

	controller := NewShutdownController(ShutdownControllerConfig{
		Runtime:        runtime,
		PolicyResolver: resolver,
		Metrics:        metrics,
		Logger:         logger,
	})

	// Launch a sandbox
	req := &domain.SandboxRequest{
		ID:       "timeout-test-1",
		Template: "test-template",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	// Terminate - should timeout and force kill
	result, err := controller.RequestTermination(ctx, &TerminationRequest{
		SandboxID:  req.ID,
		TemplateID: req.Template,
		Reason:     ReasonTimeLimit,
	})
	require.Error(t, err)
	require.Equal(t, PhaseKilled, result.Phase)
	require.Contains(t, result.ErrorMessage, "grace period exceeded")
}

func TestShutdownController_NilRequest(t *testing.T) {
	controller := NewShutdownController(ShutdownControllerConfig{})

	result, err := controller.RequestTermination(context.Background(), nil)
	require.Error(t, err)
	require.Nil(t, result)
}

func TestMapToTerminationReason(t *testing.T) {
	tests := []struct {
		input    string
		expected TerminationReason
	}{
		{"runtime_exceeded", ReasonTimeLimit},
		{"time_limit", ReasonTimeLimit},
		{"memory_exceeded", ReasonResourceLimit},
		{"resource_limit", ReasonResourceLimit},
		{"network_egress_exceeded", ReasonNetworkViolation},
		{"network_ingress_exceeded", ReasonNetworkViolation},
		{"banned_ip_attempts_exceeded", ReasonNetworkViolation},
		{"policy_breach", ReasonPolicyBreach},
		{"unknown_reason", ReasonPolicyBreach}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapToTerminationReason(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
