package tartarus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type MockRuntime struct {
	Logger   *slog.Logger
	runs     map[domain.SandboxID]*domain.SandboxRun
	configs  map[domain.SandboxID]VMConfig
	requests map[domain.SandboxID]*domain.SandboxRequest
	paused   map[domain.SandboxID]bool
	waiters  map[domain.SandboxID]chan struct{}
	// ShutdownDelay allows tests to simulate slow graceful exits.
	ShutdownDelay time.Duration
	mu            sync.RWMutex
}

func NewMockRuntime(logger *slog.Logger) *MockRuntime {
	return &MockRuntime{
		Logger:   logger,
		runs:     make(map[domain.SandboxID]*domain.SandboxRun),
		configs:  make(map[domain.SandboxID]VMConfig),
		requests: make(map[domain.SandboxID]*domain.SandboxRequest),
		paused:   make(map[domain.SandboxID]bool),
		waiters:  make(map[domain.SandboxID]chan struct{}),
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

	reqCopy := *req
	run := &domain.SandboxRun{
		ID:        req.ID,
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
	r.requests[run.ID] = &reqCopy
	r.waiters[run.ID] = make(chan struct{})
	r.mu.Unlock()

	return run, nil
}

func (r *MockRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if run, ok := r.runs[id]; ok {
		// Mock memory usage: 50% of allocated
		if cfg, ok := r.configs[id]; ok {
			run.MemoryUsage = domain.Megabytes(cfg.MemoryMB / 2)
		}
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
	delete(r.requests, id)
	r.closeWaiter(id)
	delete(r.paused, id)
	r.mu.Unlock()
	return nil
}

func (m *MockRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
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

func (r *MockRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	r.mu.RLock()
	ch, ok := r.waiters[id]
	r.mu.RUnlock()
	if !ok {
		return errors.New("sandbox not found")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}

func (r *MockRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string) error {
	r.mu.RLock()
	_, ok := r.runs[id]
	r.mu.RUnlock()
	if !ok {
		return errors.New("sandbox not found")
	}
	r.Logger.Info("Executing command in sandbox", "id", id, "cmd", cmd)
	// Simulate some execution, maybe a delay
	select {
	case <-time.After(100 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *MockRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runs[id]; !ok {
		return errors.New("sandbox not found")
	}
	r.paused[id] = true
	return nil
}

func (r *MockRuntime) Resume(ctx context.Context, id domain.SandboxID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runs[id]; !ok {
		return errors.New("sandbox not found")
	}
	delete(r.paused, id)
	return nil
}

func (r *MockRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	r.mu.RLock()
	_, ok := r.runs[id]
	r.mu.RUnlock()
	if !ok {
		return errors.New("sandbox not found")
	}

	if err := os.MkdirAll(filepath.Dir(memPath), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
		return err
	}

	content := fmt.Sprintf("snapshot of %s at %s", id, time.Now().Format(time.RFC3339Nano))
	if err := os.WriteFile(memPath, []byte(content), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(diskPath, []byte(content+"-disk"), 0644); err != nil {
		return err
	}
	return nil
}

func (r *MockRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	r.mu.RLock()
	_, ok := r.waiters[id]
	r.mu.RUnlock()
	if !ok {
		return errors.New("sandbox not found")
	}

	delay := r.ShutdownDelay
	if delay == 0 {
		delay = 10 * time.Millisecond
	}

	go func() {
		select {
		case <-time.After(delay):
			r.mu.Lock()
			if run, ok := r.runs[id]; ok {
				exit := 0
				run.ExitCode = &exit
				run.Status = domain.RunStatusSucceeded
				run.UpdatedAt = time.Now()
			}
			r.closeWaiter(id)
			r.mu.Unlock()
		case <-ctx.Done():
		}
	}()

	return nil
}

func (r *MockRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (VMConfig, *domain.SandboxRequest, error) {
	r.mu.RLock()
	cfg, okCfg := r.configs[id]
	req, okReq := r.requests[id]
	r.mu.RUnlock()
	if !okCfg || !okReq {
		return VMConfig{}, nil, errors.New("sandbox not found")
	}
	reqCopy := *req
	return cfg, &reqCopy, nil
}

func (r *MockRuntime) closeWaiter(id domain.SandboxID) {
	if ch, ok := r.waiters[id]; ok {
		select {
		case <-ch:
		default:
			close(ch)
		}
		delete(r.waiters, id)
	}
}
