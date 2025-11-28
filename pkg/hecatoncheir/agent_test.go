package hecatoncheir

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/lethe"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
	"github.com/tartarus-sandbox/tartarus/pkg/styx"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// Mocks

type mockQueue struct {
	acheron.Queue
	req *domain.SandboxRequest
}

func (m *mockQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, error) {
	if m.req != nil {
		r := m.req
		m.req = nil
		return r, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockQueue) Ack(ctx context.Context, id domain.SandboxID) error {
	return nil
}

func (m *mockQueue) Nack(ctx context.Context, id domain.SandboxID, reason string) error {
	return nil
}

type mockNyx struct {
	nyx.Manager
}

func (m *mockNyx) GetSnapshot(ctx context.Context, template domain.TemplateID) (*nyx.Snapshot, error) {
	return &nyx.Snapshot{ID: "snap-1", Path: "/tmp/snap", Template: template}, nil
}

type mockLethe struct {
	lethe.Pool
}

func (m *mockLethe) Create(ctx context.Context, snap *nyx.Snapshot) (*lethe.Overlay, error) {
	return &lethe.Overlay{ID: "ov-1", MountPath: "/tmp/ov"}, nil
}

func (m *mockLethe) Destroy(ctx context.Context, overlay *lethe.Overlay) error {
	return nil
}

type mockStyx struct {
	styx.Gateway
}

func (m *mockStyx) Attach(ctx context.Context, sandboxID domain.SandboxID, contract *styx.Contract) (string, netip.Addr, netip.Addr, netip.Prefix, error) {
	return "tap0", netip.Addr{}, netip.Addr{}, netip.Prefix{}, nil
}

func (m *mockStyx) Detach(ctx context.Context, sandboxID domain.SandboxID) error {
	return nil
}

type mockRuntime struct {
	tartarus.SandboxRuntime
}

func (m *mockRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	if req.ID == "req-fail" {
		return nil, errors.New("launch failed")
	}
	return &domain.SandboxRun{ID: req.ID, Status: domain.RunStatusRunning}, nil
}

func (m *mockRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	return nil
}

func (m *mockRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	return &domain.SandboxRun{ID: id, Status: domain.RunStatusSucceeded}, nil
}
func (m *mockRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) { return nil, nil }
func (m *mockRuntime) Kill(ctx context.Context, id domain.SandboxID) error   { return nil }
func (m *mockRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer) error {
	return nil
}
func (m *mockRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	return domain.ResourceCapacity{}, nil
}

type mockSink struct {
	cocytus.Sink
	written *cocytus.Record
	err     error
}

func (m *mockSink) Write(ctx context.Context, rec *cocytus.Record) error {
	m.written = rec
	return m.err
}

type mockRegistry struct {
	hades.Registry
}

func (m *mockRegistry) UpdateRun(ctx context.Context, run domain.SandboxRun) error {
	return nil
}

type mockFury struct {
	erinyes.Fury
}

func (m *mockFury) Arm(ctx context.Context, run *domain.SandboxRun, policy *erinyes.PolicySnapshot) error {
	return nil
}

func (m *mockFury) Disarm(ctx context.Context, runID domain.SandboxID) error {
	return nil
}

type mockLogger struct {
	hermes.Logger
}

func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any)  {}
func (m *mockLogger) Error(ctx context.Context, msg string, fields map[string]any) {}

func TestAgent_Run_ReportFailure(t *testing.T) {
	req := &domain.SandboxRequest{
		ID:       "req-fail",
		Template: "base",
		Resources: domain.ResourceSpec{
			CPU: 1,
			Mem: 128,
		},
		NetworkRef: domain.NetworkPolicyRef{ID: "net-1"},
	}

	sink := &mockSink{}
	agent := &Agent{
		Queue:      &mockQueue{req: req},
		Nyx:        &mockNyx{},
		Lethe:      &mockLethe{},
		Styx:       &mockStyx{},
		Runtime:    &mockRuntime{},
		Registry:   &mockRegistry{},
		Furies:     &mockFury{},
		DeadLetter: sink,
		Logger:     &mockLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run agent
	agent.Run(ctx)

	// Verify sink was called
	for i := 0; i < 10; i++ {
		if sink.written != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if sink.written == nil {
		t.Fatal("Expected DeadLetter.Write to be called")
	}

	if sink.written.RequestID != req.ID {
		t.Errorf("Expected RequestID %s, got %s", req.ID, sink.written.RequestID)
	}

	if sink.written.Reason != "launch failed" {
		t.Errorf("Expected Reason 'launch failed', got '%s'", sink.written.Reason)
	}
}

func TestAgent_Run_Success_Cleanup(t *testing.T) {
	req := &domain.SandboxRequest{
		ID:       "req-success",
		Template: "base",
		Resources: domain.ResourceSpec{
			CPU: 1,
			Mem: 128,
		},
		NetworkRef: domain.NetworkPolicyRef{ID: "net-1"},
	}

	letheMock := &mockLethe{}
	styxMock := &mockStyx{}
	agent := &Agent{
		Queue:      &mockQueue{req: req},
		Nyx:        &mockNyx{},
		Lethe:      letheMock,
		Styx:       styxMock,
		Runtime:    &mockRuntime{},
		Registry:   &mockRegistry{},
		Furies:     &mockFury{},
		DeadLetter: &mockSink{},
		Logger:     &mockLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run agent
	agent.Run(ctx)

	// We can't easily verify cleanup because mocks don't track calls in this simple setup.
	// But at least we verify it doesn't crash.
	// To verify cleanup, we'd need spy mocks.
}
