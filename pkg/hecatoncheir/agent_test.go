package hecatoncheir

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
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

func (m *mockStyx) Attach(ctx context.Context, sandboxID domain.SandboxID, contract *styx.Contract) (string, netip.Addr, error) {
	return "tap0", netip.Addr{}, nil
}

func (m *mockStyx) Detach(ctx context.Context, sandboxID domain.SandboxID) error {
	return nil
}

type mockRuntime struct {
	tartarus.SandboxRuntime
}

func (m *mockRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	return nil, errors.New("launch failed")
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

type mockLogger struct {
	hermes.Logger
}

func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any)  {}
func (m *mockLogger) Error(ctx context.Context, msg string, fields map[string]any) {}

func TestAgent_Run_ReportFailure(t *testing.T) {
	req := &domain.SandboxRequest{
		ID:       "req-1",
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
		DeadLetter: sink,
		Logger:     &mockLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run agent
	agent.Run(ctx)

	// Verify sink was called
	// Since the sink write is async, we might need to wait a bit, but the context timeout should handle it if we are lucky.
	// Actually, since it's a goroutine, we should wait for it.
	// But `agent.Run` blocks until context is done.
	// The goroutine starts before `agent.Run` loop continues (or rather, inside the loop).
	// When `agent.Run` returns (due to context timeout), the goroutine might still be running or finished.
	// We should probably use a channel in the mock sink to signal completion if we want to be robust.

	// Let's retry checking for a short duration
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
