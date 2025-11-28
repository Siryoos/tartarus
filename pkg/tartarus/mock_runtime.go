package tartarus

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type MockRuntime struct {
	Logger *slog.Logger
}

func NewMockRuntime(logger *slog.Logger) *MockRuntime {
	return &MockRuntime{Logger: logger}
}

func (r *MockRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	r.Logger.Info("Launching sandbox", "id", req.ID, "template", req.Template)

	// Simulate startup delay
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &domain.SandboxRun{
		ID:        domain.SandboxID("run-" + string(req.ID)),
		RequestID: req.ID,
		NodeID:    "mock-node",
		Template:  req.Template,
		Status:    domain.RunStatusRunning,
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (r *MockRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	return &domain.SandboxRun{
		ID:     id,
		Status: domain.RunStatusRunning,
	}, nil
}

func (r *MockRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	r.Logger.Info("Killing sandbox", "id", id)
	return nil
}

func (r *MockRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer) error {
	_, err := w.Write([]byte("mock logs for " + string(id) + "\n"))
	return err
}
