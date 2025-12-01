package kampe

import (
	"context"
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
	adapter, _ := NewDockerAdapter("/var/run/docker.sock")
	ctx := context.Background()

	tests := []struct {
		name          string
		containerID   string
		wantRisk      RiskLevel
		wantChangeLen int
	}{
		{
			name:          "Simple Container",
			containerID:   "simple-container",
			wantRisk:      RiskLevelLow,
			wantChangeLen: 0,
		},
		{
			name:          "Complex Container",
			containerID:   "complex-container",
			wantRisk:      RiskLevelHigh,
			wantChangeLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := adapter.MigrateToMicroVM(ctx, tt.containerID)
			if err != nil {
				t.Fatalf("MigrateToMicroVM failed: %v", err)
			}
			if plan.RiskLevel != tt.wantRisk {
				t.Errorf("RiskLevel = %v, want %v", plan.RiskLevel, tt.wantRisk)
			}
			if len(plan.RequiredChanges) != tt.wantChangeLen {
				t.Errorf("RequiredChanges len = %d, want %d", len(plan.RequiredChanges), tt.wantChangeLen)
			}
		})
	}
}

func TestContainerdAdapter_MigrateToMicroVM(t *testing.T) {
	adapter, _ := NewContainerdAdapter("/run/containerd/containerd.sock")
	ctx := context.Background()

	plan, err := adapter.MigrateToMicroVM(ctx, "any-container")
	if err != nil {
		t.Fatalf("MigrateToMicroVM failed: %v", err)
	}
	if plan.RiskLevel != RiskLevelLow {
		t.Errorf("RiskLevel = %v, want %v", plan.RiskLevel, RiskLevelLow)
	}
	if plan.TargetTemplate != "microvm-containerd-compatible" {
		t.Errorf("TargetTemplate = %v, want %v", plan.TargetTemplate, "microvm-containerd-compatible")
	}
}

func TestGVisorAdapter_MigrateToMicroVM(t *testing.T) {
	adapter, _ := NewGVisorAdapter("/run/gvisor/gvisor.sock")
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
		a, _ := NewDockerAdapter("")
		s, err := a.ExportState(ctx, "c1")
		if err != nil {
			t.Fatal(err)
		}
		if s.ID != "c1" {
			t.Errorf("ID = %v, want c1", s.ID)
		}
	})

	t.Run("Containerd", func(t *testing.T) {
		a, _ := NewContainerdAdapter("")
		s, err := a.ExportState(ctx, "c2")
		if err != nil {
			t.Fatal(err)
		}
		if s.ID != "c2" {
			t.Errorf("ID = %v, want c2", s.ID)
		}
	})

	t.Run("GVisor", func(t *testing.T) {
		a, _ := NewGVisorAdapter("")
		s, err := a.ExportState(ctx, "c3")
		if err != nil {
			t.Fatal(err)
		}
		if s.ID != "c3" {
			t.Errorf("ID = %v, want c3", s.ID)
		}
	})
}
