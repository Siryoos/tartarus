package phlegethon

import (
	"testing"
	"time"
)

func TestHeatClassifier_Classify(t *testing.T) {
	classifier := NewHeatClassifier()
	classifier.AddHint("heavy-template", HeatInferno)

	tests := []struct {
		name string
		req  *SandboxRequest
		want HeatLevel
	}{
		{
			name: "Explicit Hint",
			req: &SandboxRequest{
				HeatHint: HeatWarm,
			},
			want: HeatWarm,
		},
		{
			name: "Template Hint",
			req: &SandboxRequest{
				TemplateID: "heavy-template",
			},
			want: HeatInferno,
		},
		{
			name: "Inferno by Duration",
			req: &SandboxRequest{
				MaxDuration: 15 * time.Minute,
			},
			want: HeatInferno,
		},
		{
			name: "Inferno by CPU",
			req: &SandboxRequest{
				CPUCores: 4,
			},
			want: HeatInferno,
		},
		{
			name: "Hot by Duration",
			req: &SandboxRequest{
				MaxDuration: 5 * time.Minute,
			},
			want: HeatHot,
		},
		{
			name: "Hot by CPU",
			req: &SandboxRequest{
				CPUCores: 2,
			},
			want: HeatHot,
		},
		{
			name: "Warm by Duration",
			req: &SandboxRequest{
				MaxDuration: 1 * time.Minute,
			},
			want: HeatWarm,
		},
		{
			name: "Cold Default",
			req: &SandboxRequest{
				MaxDuration: 10 * time.Second,
				CPUCores:    1,
			},
			want: HeatCold,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifier.Classify(tt.req); got != tt.want {
				t.Errorf("HeatClassifier.Classify() = %v, want %v", got, tt.want)
			}
		})
	}
}
