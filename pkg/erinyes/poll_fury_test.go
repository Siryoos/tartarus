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

// MockNetworkStatsProvider for testing
type MockNetworkStatsProvider struct {
	RxBytes   int64
	TxBytes   int64
	DropCount int
	Err       error
}

func (m *MockNetworkStatsProvider) GetInterfaceStats(ctx context.Context, ifaceName string) (int64, int64, error) {
	return m.RxBytes, m.TxBytes, m.Err
}

func (m *MockNetworkStatsProvider) GetDropCount(ctx context.Context, tapName string) (int, error) {
	return m.DropCount, m.Err
}

func TestPollFury_MemoryEnforcement(t *testing.T) {
	// Setup
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewNoopMetrics()
	runtime := tartarus.NewMockRuntime(slog.Default())
	networkStats := &MockNetworkStatsProvider{}
	fury := NewPollFury(runtime, logger, metrics, networkStats, 10*time.Millisecond)

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

func TestPollFury_NetworkEnforcement(t *testing.T) {
	// Setup
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewNoopMetrics()
	runtime := tartarus.NewMockRuntime(slog.Default())
	networkStats := &MockNetworkStatsProvider{}
	fury := NewPollFury(runtime, logger, metrics, networkStats, 10*time.Millisecond)

	ctx := context.Background()

	// Create a run with TAP device
	req := &domain.SandboxRequest{
		ID:       "test-net-limit",
		Template: "test-template",
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 100,
		},
	}
	cfg := tartarus.VMConfig{
		CPUs:      1,
		MemoryMB:  100,
		TapDevice: "tap-test",
	}

	run, err := runtime.Launch(ctx, req, cfg)
	if err != nil {
		t.Fatalf("Failed to launch sandbox: %v", err)
	}

	// Case 1: Egress Limit Exceeded
	networkStats.RxBytes = 200 * 1024 * 1024 // 200MB (Host RX = VM Egress)
	policyEgress := &PolicySnapshot{
		MaxNetworkEgressBytes: 100 * 1024 * 1024, // 100MB
		KillOnBreach:          true,
	}

	if err := fury.Arm(ctx, run, policyEgress); err != nil {
		t.Fatalf("Failed to arm fury: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_, err = runtime.Inspect(ctx, run.ID)
	if err == nil {
		t.Error("Expected error (sandbox killed due to egress), got nil")
	}

	// Relaunch for next test
	run, _ = runtime.Launch(ctx, req, cfg)

	// Case 2: Ingress Limit Exceeded
	networkStats.RxBytes = 0
	networkStats.TxBytes = 200 * 1024 * 1024 // 200MB (Host TX = VM Ingress)
	policyIngress := &PolicySnapshot{
		MaxNetworkIngressBytes: 100 * 1024 * 1024, // 100MB
		KillOnBreach:           true,
	}

	if err := fury.Arm(ctx, run, policyIngress); err != nil {
		t.Fatalf("Failed to arm fury: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_, err = runtime.Inspect(ctx, run.ID)
	if err == nil {
		t.Error("Expected error (sandbox killed due to ingress), got nil")
	}

	// Relaunch for next test
	run, _ = runtime.Launch(ctx, req, cfg)

	// Case 3: Banned IP Attempts Exceeded
	networkStats.TxBytes = 0
	networkStats.DropCount = 10
	policyBanned := &PolicySnapshot{
		MaxBannedIPAttempts: 5,
		KillOnBreach:        true,
	}

	if err := fury.Arm(ctx, run, policyBanned); err != nil {
		t.Fatalf("Failed to arm fury: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_, err = runtime.Inspect(ctx, run.ID)
	if err == nil {
		t.Error("Expected error (sandbox killed due to banned IP attempts), got nil")
	}
}
