package hecatoncheir

import (
	"context"
	"io"
	"testing"
	"time"

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

func (m *MockControlListener) PublishSandboxes(ctx context.Context, requestID string, sandboxes []domain.SandboxRun) error {
	args := m.Called(ctx, requestID, sandboxes)
	return args.Error(0)
}

func (m *MockControlListener) PublishExecOutput(ctx context.Context, sandboxID domain.SandboxID, requestID string, output []byte) error {
	args := m.Called(ctx, sandboxID, requestID, output)
	return args.Error(0)
}

func (m *MockControlListener) SubscribeStdin(ctx context.Context, requestID string) (<-chan []byte, error) {
	args := m.Called(ctx, requestID)
	return args.Get(0).(<-chan []byte), args.Error(1)
}

// MockRuntime
type MockRuntime struct {
	mock.Mock
}

func (m *MockRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	args := m.Called(ctx, req, cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SandboxRun), args.Error(1)
}
func (m *MockRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	args := m.Called(ctx, id)
	// Handle nil return safely if needed, but for now strict casting
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SandboxRun), args.Error(1)
}
func (m *MockRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) { return nil, nil }
func (m *MockRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockRuntime) Resume(ctx context.Context, id domain.SandboxID) error { return nil }
func (m *MockRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	args := m.Called(ctx, id, memPath, diskPath)
	return args.Error(0)
}
func (m *MockRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (tartarus.VMConfig, *domain.SandboxRequest, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(tartarus.VMConfig), args.Get(1).(*domain.SandboxRequest), args.Error(2)
}
func (m *MockRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	args := m.Called(ctx, id, w, follow)
	return args.Error(0)
}
func (m *MockRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	return domain.ResourceCapacity{}, nil
}
func (m *MockRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	args := m.Called(ctx, id, cmd, stdout, stderr)
	return args.Error(0)
}
func (m *MockRuntime) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	args := m.Called(ctx, id, cmd, stdin, stdout, stderr)
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

	mockRuntime.On("Exec", mock.Anything, sandboxID, cmd, mock.Anything, mock.Anything).Return(nil)

	// Create channel and send message
	ch := make(chan ControlMessage)
	go func() {
		ch <- ControlMessage{
			Type:      ControlMessageExec,
			SandboxID: sandboxID,
			Args:      append([]string{"req-1"}, cmd...),
		}
		close(ch)
	}()

	// Run control loop
	agent.controlLoop(ctx, ch)

	// Wait for goroutines to finish
	time.Sleep(100 * time.Millisecond)

	// Verify
	mockRuntime.AssertExpectations(t)
}

func TestAgent_ControlLoop_Logs(t *testing.T) {
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

	// Expect StreamLogs to be called with follow=true
	mockRuntime.On("StreamLogs", mock.Anything, sandboxID, mock.Anything, true).Return(nil).Run(func(args mock.Arguments) {
		// Simulate log streaming
		w := args.Get(2).(io.Writer)
		w.Write([]byte("mock logs"))
	})

	// Expect PublishLogs to be called
	mockListener.On("PublishLogs", mock.Anything, sandboxID, mock.MatchedBy(func(logs []byte) bool {
		return string(logs) == "mock logs"
	})).Return(nil)

	// Create channel and send message
	ch := make(chan ControlMessage)
	go func() {
		ch <- ControlMessage{
			Type:      ControlMessageLogs,
			SandboxID: sandboxID,
			Args:      []string{"true"}, // follow=true
		}
		close(ch)
	}()

	// Run control loop
	agent.controlLoop(ctx, ch)

	// Wait for goroutines to finish
	time.Sleep(100 * time.Millisecond)

	// Verify
	mockRuntime.AssertExpectations(t)
}
