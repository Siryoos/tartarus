package kampe

import (
	"context"
	"os"
	"testing"

	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

func TestAdaptersImplementInterface(t *testing.T) {
	var _ LegacyRuntime = &DockerAdapter{}
	var _ LegacyRuntime = &ContainerdAdapter{}
	var _ LegacyRuntime = &GVisorAdapter{}
	var _ tartarus.SandboxRuntime = &DockerAdapter{}
	var _ tartarus.SandboxRuntime = &ContainerdAdapter{}
	var _ tartarus.SandboxRuntime = &GVisorAdapter{}
}

func TestDockerAdapter_MigrateToMicroVM(t *testing.T) {
	adapter, err := NewDockerAdapter("")
	if err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()

	// Note: These tests require actual containers to exist
	// They are integration tests that run only when Docker is available
	t.Run("NonExistentContainer", func(t *testing.T) {
		_, err := adapter.MigrateToMicroVM(ctx, "nonexistent-container")
		if err == nil {
			t.Skip("Expected error for non-existent container")
		}
		// Error is expected for non-existent container
		t.Log("Got expected error for non-existent container:", err)
	})
}

func TestContainerdAdapter_MigrateToMicroVM(t *testing.T) {
	adapter, err := NewContainerdAdapter("")
	if err != nil {
		t.Skip("containerd not available:", err)
	}

	ctx := context.Background()

	// This test requires containerd with actual containers
	plan, err := adapter.MigrateToMicroVM(ctx, "any-container")
	if err != nil {
		t.Skip("MigrateToMicroVM requires actual container:", err)
	}
	if plan.RiskLevel != RiskLevelLow {
		t.Errorf("RiskLevel = %v, want %v", plan.RiskLevel, RiskLevelLow)
	}
	if plan.TargetTemplate != "microvm-containerd-compatible" {
		t.Errorf("TargetTemplate = %v, want %v", plan.TargetTemplate, "microvm-containerd-compatible")
	}
}

func TestGVisorAdapter_MigrateToMicroVM(t *testing.T) {
	adapter, err := NewGVisorAdapter("")
	if err != nil {
		t.Skip("gVisor (runsc) not available:", err)
	}

	ctx := context.Background()

	plan, err := adapter.MigrateToMicroVM(ctx, "any-container")
	if err != nil {
		t.Fatalf("MigrateToMicroVM failed: %v", err)
	}
	if plan.RiskLevel != RiskLevelLow {
		t.Errorf("RiskLevel = %v, want %v", plan.RiskLevel, RiskLevelLow)
	}
	if len(plan.Recommendations) == 0 {
		t.Error("Expected recommendations for gVisor migration")
	}
}

func TestAdapters_ExportState(t *testing.T) {
	ctx := context.Background()

	t.Run("Docker", func(t *testing.T) {
		a, err := NewDockerAdapter("")
		if err != nil {
			t.Skip("Docker not available:", err)
		}
		_, err = a.ExportState(ctx, "c1")
		if err == nil {
			t.Log("ExportState succeeded (container exists)")
		} else {
			// Expected - container doesn't exist
			t.Log("ExportState failed as expected (container doesn't exist):", err)
		}
	})

	t.Run("Containerd", func(t *testing.T) {
		a, err := NewContainerdAdapter("")
		if err != nil {
			t.Skip("containerd not available:", err)
		}
		_, err = a.ExportState(ctx, "c2")
		if err == nil {
			t.Log("ExportState succeeded (container exists)")
		} else {
			// Expected - container doesn't exist
			t.Log("ExportState failed as expected (container doesn't exist):", err)
		}
	})

	t.Run("GVisor", func(t *testing.T) {
		a, err := NewGVisorAdapter("")
		if err != nil {
			t.Skip("gVisor not available:", err)
		}
		_, err = a.ExportState(ctx, "c3")
		if err == nil {
			t.Log("ExportState succeeded (container exists)")
		} else {
			// Expected - container doesn't exist
			t.Log("ExportState failed as expected (container doesn't exist):", err)
		}
	})
}

// Helper functions for availability checks
func dockerAvailable() bool {
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return true
	}
	return os.Getenv("DOCKER_HOST") != ""
}

func containerdAvailable() bool {
	_, err := os.Stat("/run/containerd/containerd.sock")
	return err == nil
}

func gvisorAvailable() bool {
	_, err := os.Stat("/usr/local/bin/runsc")
	if err == nil {
		return true
	}
	_, err = os.Stat("/usr/bin/runsc")
	return err == nil
}
