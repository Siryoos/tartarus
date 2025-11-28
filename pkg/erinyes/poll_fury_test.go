package erinyes

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

func TestPollFury_MemoryEnforcement(t *testing.T) {
	// Setup
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewNoopMetrics()
	runtime := tartarus.NewMockRuntime(slog.Default())
	fury := NewPollFury(runtime, logger, metrics, 10*time.Millisecond)

	ctx := context.Background()

	// Create a run
	req := &domain.SandboxRequest{
		ID:       "test-mem-limit",
		Template: "test-template",
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 100, // 100MB
		},
	}
	cfg := tartarus.VMConfig{
		CPUs:     1,
		MemoryMB: 100,
	}

	run, err := runtime.Launch(ctx, req, cfg)
	if err != nil {
		t.Fatalf("Failed to launch sandbox: %v", err)
	}

	// MockRuntime sets usage to 50% of allocated (50MB)
	// Case 1: Limit is 60MB (should NOT kill)
	policySafe := &PolicySnapshot{
		MaxMemory:    60,
		KillOnBreach: true,
	}

	if err := fury.Arm(ctx, run, policySafe); err != nil {
		t.Fatalf("Failed to arm fury: %v", err)
	}

	// Wait for a few ticks
	time.Sleep(50 * time.Millisecond)

	// Check status
	status, err := runtime.Inspect(ctx, run.ID)
	if err != nil {
		t.Fatalf("Failed to inspect: %v", err)
	}
	if status.Status != domain.RunStatusRunning {
		t.Errorf("Expected status RUNNING, got %s", status.Status)
	}

	fury.Disarm(ctx, run.ID)

	// Case 2: Limit is 40MB (should KILL)
	policyStrict := &PolicySnapshot{
		MaxMemory:    40,
		KillOnBreach: true,
	}

	if err := fury.Arm(ctx, run, policyStrict); err != nil {
		t.Fatalf("Failed to arm fury: %v", err)
	}

	// Wait for a few ticks
	time.Sleep(50 * time.Millisecond)

	// Check status - should be gone (MockRuntime deletes on Kill)
	// Or Inspect returns error "sandbox not found"
	_, err = runtime.Inspect(ctx, run.ID)
	if err == nil {
		t.Error("Expected error (sandbox killed), got nil")
	}
}
