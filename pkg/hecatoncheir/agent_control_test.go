package hecatoncheir

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// MockControlListener
type MockControlListener struct {
	mock.Mock
}

func (m *MockControlListener) Listen(ctx context.Context) (<-chan ControlMessage, error) {
	args := m.Called(ctx)
	return args.Get(0).(<-chan ControlMessage), args.Error(1)
}

func (m *MockControlListener) PublishLogs(ctx context.Context, sandboxID domain.SandboxID, logs []byte) error {
	args := m.Called(ctx, sandboxID, logs)
	return args.Error(0)
}

// MockRuntime
type MockRuntime struct {
	mock.Mock
}

func (m *MockRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	return nil, nil
}
func (m *MockRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	return nil, nil
}
func (m *MockRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) { return nil, nil }
func (m *MockRuntime) Kill(ctx context.Context, id domain.SandboxID) error   { return nil }
func (m *MockRuntime) Pause(ctx context.Context, id domain.SandboxID) error  { return nil }
func (m *MockRuntime) Resume(ctx context.Context, id domain.SandboxID) error { return nil }
func (m *MockRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	return nil
}
func (m *MockRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error { return nil }
func (m *MockRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (tartarus.VMConfig, *domain.SandboxRequest, error) {
	return tartarus.VMConfig{}, nil, nil
}
func (m *MockRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	return nil
}
func (m *MockRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	return domain.ResourceCapacity{}, nil
}
func (m *MockRuntime) Wait(ctx context.Context, id domain.SandboxID) error { return nil }
func (m *MockRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string) error {
	args := m.Called(ctx, id, cmd)
	return args.Error(0)
}

func TestAgent_ControlLoop_Exec(t *testing.T) {
	// Setup
	mockRuntime := new(MockRuntime)
	mockListener := new(MockControlListener)
	agent := &Agent{
		Runtime: mockRuntime,
		Control: mockListener,
		Logger:  hermes.NewSlogAdapter(),
	}

	// Expectation
	ctx := context.Background()
	sandboxID := domain.SandboxID("test-sandbox")
	cmd := []string{"ls", "-la"}

	mockRuntime.On("Exec", mock.Anything, sandboxID, cmd).Return(nil)

	// Create channel and send message
	ch := make(chan ControlMessage)
	go func() {
		ch <- ControlMessage{
			Type:      ControlMessageExec,
			SandboxID: sandboxID,
			Args:      cmd,
		}
		close(ch)
	}()

	// Run control loop
	agent.controlLoop(ctx, ch)

	// Verify
	mockRuntime.AssertExpectations(t)
}
