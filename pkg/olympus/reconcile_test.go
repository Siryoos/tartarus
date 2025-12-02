package olympus

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// ReconcileMockHades for testing
type ReconcileMockHades struct {
	Nodes []domain.NodeStatus
	Runs  map[domain.SandboxID]domain.SandboxRun
}

func (m *ReconcileMockHades) ListNodes(ctx context.Context) ([]domain.NodeStatus, error) {
	return m.Nodes, nil
}
func (m *ReconcileMockHades) GetNode(ctx context.Context, id domain.NodeID) (*domain.NodeStatus, error) {
	return nil, nil
}
func (m *ReconcileMockHades) UpdateHeartbeat(ctx context.Context, payload hades.HeartbeatPayload) error {
	return nil
}

func (m *ReconcileMockHades) UpdateRun(ctx context.Context, run domain.SandboxRun) error {
	if m.Runs == nil {
		m.Runs = make(map[domain.SandboxID]domain.SandboxRun)
	}
	m.Runs[run.ID] = run
	return nil
}

// Stub other methods to satisfy interface
func (m *ReconcileMockHades) GetRun(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	return nil, nil
}
func (m *ReconcileMockHades) ListRuns(ctx context.Context) ([]domain.SandboxRun, error) {
	return nil, nil
}
func (m *ReconcileMockHades) MarkDraining(ctx context.Context, id domain.NodeID) error { return nil }

// We need the exact signature for UpdateHeartbeat.
// It uses hades.HeartbeatPayload.
// I'll skip implementing the full interface by using a struct that embeds the interface or just implementing the methods I need and casting?
// No, Manager expects hades.Registry.
// I will implement a minimal MockHades that satisfies the interface.
// I need to import "github.com/tartarus-sandbox/tartarus/pkg/hades"

// ReconcileMockControlPlane for testing
type ReconcileMockControlPlane struct {
	Sandboxes map[domain.NodeID][]domain.SandboxRun
}

func (m *ReconcileMockControlPlane) ListSandboxes(ctx context.Context, nodeID domain.NodeID) ([]domain.SandboxRun, error) {
	if runs, ok := m.Sandboxes[nodeID]; ok {
		return runs, nil
	}
	return nil, errors.New("node not found or error")
}

// Stubs
func (m *ReconcileMockControlPlane) Kill(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}
func (m *ReconcileMockControlPlane) StreamLogs(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, w io.Writer, follow bool) error {
	return nil
}
func (m *ReconcileMockControlPlane) Hibernate(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}
func (m *ReconcileMockControlPlane) Wake(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}
func (m *ReconcileMockControlPlane) Snapshot(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}
func (m *ReconcileMockControlPlane) Exec(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	return nil
}
func (m *ReconcileMockControlPlane) ExecInteractive(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return nil
}

func TestReconcile(t *testing.T) {
	// Setup
	node1 := domain.NodeID("node-1")
	node2 := domain.NodeID("node-2")

	hadesMock := &ReconcileMockHades{
		Nodes: []domain.NodeStatus{
			{NodeInfo: domain.NodeInfo{ID: node1}},
			{NodeInfo: domain.NodeInfo{ID: node2}},
		},
		Runs: make(map[domain.SandboxID]domain.SandboxRun),
	}

	run1 := domain.SandboxRun{ID: "run-1", NodeID: node1, Status: domain.RunStatusRunning}
	run2 := domain.SandboxRun{ID: "run-2", NodeID: node2, Status: domain.RunStatusRunning}

	controlMock := &ReconcileMockControlPlane{
		Sandboxes: map[domain.NodeID][]domain.SandboxRun{
			node1: {run1},
			node2: {run2},
		},
	}

	manager := &Manager{
		Hades:   hadesMock,
		Control: controlMock,
		Logger:  hermes.NewSlogAdapter(),
	}

	// Execute
	err := manager.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify
	if len(hadesMock.Runs) != 2 {
		t.Errorf("Expected 2 runs in Hades, got %d", len(hadesMock.Runs))
	}
	if _, ok := hadesMock.Runs["run-1"]; !ok {
		t.Error("Run 1 missing from Hades")
	}
	if _, ok := hadesMock.Runs["run-2"]; !ok {
		t.Error("Run 2 missing from Hades")
	}
}
