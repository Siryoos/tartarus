//go:build integration
// +build integration

package kampe

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// TestParitySuite runs the full parity test suite
func TestParitySuite(t *testing.T) {
	harness := setupParityHarness(t)
	if len(harness.Runtimes) == 0 {
		t.Skip("No runtimes available for testing")
	}

	t.Run("suite", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		// Use t.Cleanup to ensure cancel is called after all parallel tests in this group finish
		t.Cleanup(cancel)

		for _, tc := range StandardParityTests() {
			tc := tc // capture range variable
			t.Run(tc.Name, func(t *testing.T) {
				t.Parallel()

				results, err := harness.RunTest(ctx, tc)
				if err != nil {
					t.Fatalf("Failed to run test: %v", err)
				}

				harness.Compare(t, results, tc.ExpectedBehavior)
			})
		}
	})
}

// TestDockerAdapter_Lifecycle tests Docker adapter lifecycle operations
func TestDockerAdapter_Lifecycle(t *testing.T) {
	if os.Getenv("DOCKER_HOST") == "" && !dockerAvailable() {
		t.Skip("Docker not available")
	}

	adapter, err := NewDockerAdapter("")
	if err != nil {
		t.Skipf("Failed to create Docker adapter: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := &domain.SandboxRequest{
		ID:       "test-docker-lifecycle",
		Template: "alpine:latest",
		Command:  []string{"sleep", "5"},
		Env:      map[string]string{"TEST": "value"},
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 128,
		},
	}

	// Test Launch
	run, err := adapter.Launch(ctx, req, tartarus.VMConfig{})
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	if run.Status != domain.RunStatusRunning {
		t.Errorf("Expected status RUNNING, got %s", run.Status)
	}

	// Test Inspect
	run, err = adapter.Inspect(ctx, req.ID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if run.Status != domain.RunStatusRunning {
		t.Errorf("Expected status RUNNING, got %s", run.Status)
	}

	// Test List
	runs, err := adapter.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	found := false
	for _, r := range runs {
		if r.ID == req.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Sandbox not found in list")
	}

	// Test Pause/Resume
	if err := adapter.Pause(ctx, req.ID); err != nil {
		t.Errorf("Pause failed: %v", err)
	}
	if err := adapter.Resume(ctx, req.ID); err != nil {
		t.Errorf("Resume failed: %v", err)
	}

	// Test Kill
	if err := adapter.Kill(ctx, req.ID); err != nil {
		t.Errorf("Kill failed: %v", err)
	}

	// Verify cleanup
	_, err = adapter.Inspect(ctx, req.ID)
	if err == nil {
		t.Error("Expected error after kill, got nil")
	}
}

// TestDockerAdapter_ExitCodes tests that exit codes are captured correctly
func TestDockerAdapter_ExitCodes(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	adapter, err := NewDockerAdapter("")
	if err != nil {
		t.Skipf("Failed to create Docker adapter: %v", err)
	}

	tests := []struct {
		name     string
		command  []string
		expected int
	}{
		{"exit-0", []string{"true"}, 0},
		{"exit-1", []string{"false"}, 1},
		{"exit-42", []string{"sh", "-c", "exit 42"}, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			req := &domain.SandboxRequest{
				ID:       domain.SandboxID("test-exit-" + tt.name),
				Template: "alpine:latest",
				Command:  tt.command,
				Resources: domain.ResourceSpec{
					CPU: 500,
					Mem: 64,
				},
			}

			_, err := adapter.Launch(ctx, req, tartarus.VMConfig{})
			if err != nil {
				t.Fatalf("Launch failed: %v", err)
			}
			defer adapter.Kill(context.Background(), req.ID)

			if err := adapter.Wait(ctx, req.ID); err != nil {
				t.Logf("Wait returned error: %v", err)
			}

			run, err := adapter.Inspect(ctx, req.ID)
			if err != nil {
				t.Fatalf("Inspect failed: %v", err)
			}

			if run.ExitCode == nil {
				t.Error("Exit code is nil")
			} else if *run.ExitCode != tt.expected {
				t.Errorf("Expected exit code %d, got %d", tt.expected, *run.ExitCode)
			}
		})
	}
}

// TestContainerdAdapter_Lifecycle tests containerd adapter lifecycle operations
func TestContainerdAdapter_Lifecycle(t *testing.T) {
	if !containerdAvailable() {
		t.Skip("containerd not available")
	}

	adapter, err := NewContainerdAdapter("")
	if err != nil {
		t.Skipf("Failed to create containerd adapter: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := &domain.SandboxRequest{
		ID:       "test-containerd-lifecycle",
		Template: "docker.io/library/alpine:latest",
		Command:  []string{"sleep", "5"},
		Env:      map[string]string{"TEST": "value"},
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 128,
		},
	}

	// Test Launch
	run, err := adapter.Launch(ctx, req, tartarus.VMConfig{})
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	if run.Status != domain.RunStatusRunning {
		t.Errorf("Expected status RUNNING, got %s", run.Status)
	}

	// Test Inspect
	run, err = adapter.Inspect(ctx, req.ID)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// Test Kill
	if err := adapter.Kill(ctx, req.ID); err != nil {
		t.Errorf("Kill failed: %v", err)
	}
}

// TestGVisorAdapter_Lifecycle tests gVisor adapter lifecycle operations
func TestGVisorAdapter_Lifecycle(t *testing.T) {
	if !gvisorAvailable() {
		t.Skip("gVisor (runsc) not available")
	}

	adapter, err := NewGVisorAdapter("")
	if err != nil {
		t.Skipf("Failed to create gVisor adapter: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := &domain.SandboxRequest{
		ID:       "test-gvisor-lifecycle",
		Template: "alpine:latest",
		Command:  []string{"echo", "hello"},
		Env:      map[string]string{"TEST": "value"},
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 128,
		},
	}

	// Test Launch (this will fail without a proper rootfs setup)
	_, err = adapter.Launch(ctx, req, tartarus.VMConfig{})
	if err != nil {
		t.Logf("Launch failed (expected without rootfs): %v", err)
		t.Skip("gVisor requires proper rootfs setup")
	}

	// Test Kill
	if err := adapter.Kill(ctx, req.ID); err != nil {
		t.Errorf("Kill failed: %v", err)
	}
}

// TestMigration_DockerToMicroVM tests migration planning
func TestMigration_DockerToMicroVM(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	adapter, err := NewDockerAdapter("")
	if err != nil {
		t.Skipf("Failed to create Docker adapter: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create a test container
	req := &domain.SandboxRequest{
		ID:       "test-migration",
		Template: "alpine:latest",
		Command:  []string{"sleep", "30"},
		Resources: domain.ResourceSpec{
			CPU: 500,
			Mem: 64,
		},
	}

	_, err = adapter.Launch(ctx, req, tartarus.VMConfig{})
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	defer adapter.Kill(context.Background(), req.ID)

	// Get the Docker container ID
	state, _ := adapter.getState(req.ID)

	// Test CanMigrate
	canMigrate, err := adapter.CanMigrate(ctx, state.ContainerID)
	if err != nil {
		t.Fatalf("CanMigrate failed: %v", err)
	}
	if !canMigrate {
		t.Error("Expected CanMigrate to return true")
	}

	// Test MigrateToMicroVM
	plan, err := adapter.MigrateToMicroVM(ctx, state.ContainerID)
	if err != nil {
		t.Fatalf("MigrateToMicroVM failed: %v", err)
	}
	if plan.RiskLevel != RiskLevelLow {
		t.Errorf("Expected RiskLevelLow, got %s", plan.RiskLevel)
	}
	if plan.TargetTemplate == "" {
		t.Error("TargetTemplate should not be empty")
	}

	// Test ExportState
	state2, err := adapter.ExportState(ctx, state.ContainerID)
	if err != nil {
		t.Fatalf("ExportState failed: %v", err)
	}
	if state2.Image != "alpine:latest" {
		t.Errorf("Expected image alpine:latest, got %s", state2.Image)
	}
}

// Helper functions

func setupParityHarness(t *testing.T) *ParityHarness {
	harness := NewParityHarness()

	if dockerAvailable() {
		adapter, err := NewDockerAdapter("")
		if err == nil {
			harness.AddRuntime("docker", adapter)
			t.Log("Added Docker runtime")
		}
	}

	// Only add containerd and gVisor if explicitly enabled
	// as they require more setup
	if os.Getenv("TEST_CONTAINERD") == "1" && containerdAvailable() {
		adapter, err := NewContainerdAdapter("")
		if err == nil {
			harness.AddRuntime("containerd", adapter)
			t.Log("Added containerd runtime")
		}
	}

	if os.Getenv("TEST_GVISOR") == "1" && gvisorAvailable() {
		adapter, err := NewGVisorAdapter("")
		if err == nil {
			harness.AddRuntime("gvisor", adapter)
			t.Log("Added gVisor runtime")
		}
	}

	return harness
}
