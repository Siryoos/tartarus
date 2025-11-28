package tartarus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type MockRuntime struct {
	Logger  *slog.Logger
	runs    map[domain.SandboxID]*domain.SandboxRun
	configs map[domain.SandboxID]VMConfig
	mu      sync.RWMutex
}

func NewMockRuntime(logger *slog.Logger) *MockRuntime {
	return &MockRuntime{
		Logger:  logger,
		runs:    make(map[domain.SandboxID]*domain.SandboxRun),
		configs: make(map[domain.SandboxID]VMConfig),
	}
}

func (r *MockRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	r.Logger.Info("Launching sandbox", "id", req.ID, "template", req.Template)

	// Simulate startup delay
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	run := &domain.SandboxRun{
		ID:        domain.SandboxID("run-" + string(req.ID)),
		RequestID: req.ID,
		NodeID:    "mock-node",
		Template:  req.Template,
		Status:    domain.RunStatusRunning,
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	r.mu.Lock()
	r.runs[run.ID] = run
	r.configs[run.ID] = cfg
	r.mu.Unlock()

	return run, nil
}

func (r *MockRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if run, ok := r.runs[id]; ok {
		return run, nil
	}
	return nil, errors.New("sandbox not found")
}

func (r *MockRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []domain.SandboxRun
	for _, run := range r.runs {
		list = append(list, *run)
	}
	return list, nil
}

func (r *MockRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	r.Logger.Info("Killing sandbox", "id", id)
	r.mu.Lock()
	delete(r.runs, id)
	delete(r.configs, id)
	r.mu.Unlock()
	return nil
}

func (r *MockRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer) error {
	_, err := w.Write([]byte("mock logs for " + string(id) + "\n"))
	return err
}

func (r *MockRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var cpu domain.MilliCPU
	var mem domain.Megabytes

	for _, cfg := range r.configs {
		cpu += domain.MilliCPU(cfg.CPUs * 1000)
		mem += domain.Megabytes(cfg.MemoryMB)
	}

	return domain.ResourceCapacity{
		CPU: cpu,
		Mem: mem,
		GPU: 0,
	}, nil
}
