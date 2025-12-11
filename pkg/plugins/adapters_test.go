package plugins

import (
	"context"
	"testing"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
)

// MockJudgePlugin for testing
type MockJudgePlugin struct {
	name        string
	version     string
	preVerdict  Verdict
	postClass   *Classification
	initCalled  bool
	closeCalled bool
}

func NewMockJudgePlugin(name, version string, preVerdict Verdict) *MockJudgePlugin {
	return &MockJudgePlugin{
		name:       name,
		version:    version,
		preVerdict: preVerdict,
	}
}

func (m *MockJudgePlugin) Name() string     { return m.name }
func (m *MockJudgePlugin) Version() string  { return m.version }
func (m *MockJudgePlugin) Type() PluginType { return PluginTypeJudge }
func (m *MockJudgePlugin) Init(config map[string]any) error {
	m.initCalled = true
	return nil
}
func (m *MockJudgePlugin) Close() error {
	m.closeCalled = true
	return nil
}

func (m *MockJudgePlugin) PreAdmit(ctx context.Context, req *domain.SandboxRequest) (Verdict, error) {
	return m.preVerdict, nil
}

func (m *MockJudgePlugin) PostHoc(ctx context.Context, run *domain.SandboxRun) (*Classification, error) {
	return m.postClass, nil
}

func TestJudgePluginAdapter_PreAdmit(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		pluginVerdict Verdict
		wantVerdict   judges.Verdict
	}{
		{"Accept", VerdictAccept, judges.VerdictAccept},
		{"Reject", VerdictReject, judges.VerdictReject},
		{"Quarantine", VerdictQuarantine, judges.VerdictQuarantine},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := NewMockJudgePlugin("test", "1.0.0", tt.pluginVerdict)
			adapter := NewJudgePluginAdapter(plugin)

			req := &domain.SandboxRequest{ID: "test-sandbox"}
			verdict, err := adapter.PreAdmit(ctx, req)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if verdict != tt.wantVerdict {
				t.Errorf("expected verdict %v, got %v", tt.wantVerdict, verdict)
			}
		})
	}
}

func TestJudgePluginAdapter_PostHoc(t *testing.T) {
	ctx := context.Background()

	t.Run("WithClassification", func(t *testing.T) {
		plugin := NewMockJudgePlugin("test", "1.0.0", VerdictAccept)
		plugin.postClass = &Classification{
			Verdict: VerdictQuarantine,
			Reason:  "suspicious activity",
			Labels:  map[string]string{"risk": "high"},
		}
		adapter := NewJudgePluginAdapter(plugin)

		run := &domain.SandboxRun{ID: "test-sandbox"}
		class, err := adapter.PostHoc(ctx, run)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if class == nil {
			t.Fatal("expected classification, got nil")
		}
		if class.Verdict != judges.VerdictQuarantine {
			t.Errorf("expected VerdictQuarantine, got %v", class.Verdict)
		}
		if class.Reason != "suspicious activity" {
			t.Errorf("expected reason 'suspicious activity', got '%s'", class.Reason)
		}
		if class.Labels["risk"] != "high" {
			t.Errorf("expected label risk=high, got %v", class.Labels)
		}
	})

	t.Run("NilClassification", func(t *testing.T) {
		plugin := NewMockJudgePlugin("test", "1.0.0", VerdictAccept)
		plugin.postClass = nil
		adapter := NewJudgePluginAdapter(plugin)

		run := &domain.SandboxRun{ID: "test-sandbox"}
		class, err := adapter.PostHoc(ctx, run)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if class != nil {
			t.Errorf("expected nil classification, got %v", class)
		}
	})
}

// MockFuryPlugin for testing
type MockFuryPlugin struct {
	name    string
	version string
	armed   map[domain.SandboxID]bool
}

func NewMockFuryPlugin(name, version string) *MockFuryPlugin {
	return &MockFuryPlugin{
		name:    name,
		version: version,
		armed:   make(map[domain.SandboxID]bool),
	}
}

func (m *MockFuryPlugin) Name() string                     { return m.name }
func (m *MockFuryPlugin) Version() string                  { return m.version }
func (m *MockFuryPlugin) Type() PluginType                 { return PluginTypeFury }
func (m *MockFuryPlugin) Init(config map[string]any) error { return nil }
func (m *MockFuryPlugin) Close() error                     { return nil }

func (m *MockFuryPlugin) Arm(ctx context.Context, run *domain.SandboxRun, policy *PolicySnapshot) error {
	m.armed[run.ID] = true
	return nil
}

func (m *MockFuryPlugin) Disarm(ctx context.Context, runID domain.SandboxID) error {
	delete(m.armed, runID)
	return nil
}

func TestFuryPluginAdapter_ArmDisarm(t *testing.T) {
	ctx := context.Background()

	plugin := NewMockFuryPlugin("test-fury", "1.0.0")
	adapter := NewFuryPluginAdapter(plugin)

	run := &domain.SandboxRun{ID: "test-sandbox"}
	policy := &erinyes.PolicySnapshot{
		KillOnBreach: true,
	}

	// Arm
	if err := adapter.Arm(ctx, run, policy); err != nil {
		t.Fatalf("failed to arm: %v", err)
	}
	if !plugin.armed[run.ID] {
		t.Error("expected sandbox to be armed")
	}

	// Disarm
	if err := adapter.Disarm(ctx, run.ID); err != nil {
		t.Fatalf("failed to disarm: %v", err)
	}
	if plugin.armed[run.ID] {
		t.Error("expected sandbox to be disarmed")
	}
}

func TestCompositeFury(t *testing.T) {
	ctx := context.Background()

	fury1 := NewMockFuryPlugin("fury1", "1.0.0")
	fury2 := NewMockFuryPlugin("fury2", "1.0.0")

	composite := NewCompositeFury(
		NewFuryPluginAdapter(fury1),
		NewFuryPluginAdapter(fury2),
	)

	run := &domain.SandboxRun{ID: "test-sandbox"}
	policy := &erinyes.PolicySnapshot{KillOnBreach: true}

	// Arm should arm all
	if err := composite.Arm(ctx, run, policy); err != nil {
		t.Fatalf("failed to arm composite: %v", err)
	}
	if !fury1.armed[run.ID] || !fury2.armed[run.ID] {
		t.Error("expected both furies to be armed")
	}

	// Disarm should disarm all
	if err := composite.Disarm(ctx, run.ID); err != nil {
		t.Fatalf("failed to disarm composite: %v", err)
	}
	if fury1.armed[run.ID] || fury2.armed[run.ID] {
		t.Error("expected both furies to be disarmed")
	}
}
