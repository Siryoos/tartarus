package kampe

import (
	"context"
	"testing"
)

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
