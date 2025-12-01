package tartarus

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// GVisorRuntime implements SandboxRuntime using gVisor (runsc).
// This provides strong isolation with better compatibility than pure microVMs.
type GVisorRuntime struct {
	Logger *slog.Logger

	// RunscPath is the path to the runsc binary
	RunscPath string

	// RootDir is the state directory for gVisor containers
	RootDir string

	// containers tracks active gVisor containers
	containers sync.Map // domain.SandboxID -> *gvisorContainer
}

type gvisorContainer struct {
	ID        domain.SandboxID
	Request   *domain.SandboxRequest
	Config    VMConfig
	StartedAt time.Time
	ExitCode  *int
	mu        sync.Mutex
}

// NewGVisorRuntime creates a new gVisor runtime instance.
func NewGVisorRuntime(logger *slog.Logger, runscPath, rootDir string) *GVisorRuntime {
	if runscPath == "" {
		runscPath = "/usr/local/bin/runsc"
	}
	if rootDir == "" {
		rootDir = "/var/run/gvisor"
	}

	return &GVisorRuntime{
		Logger:    logger,
		RunscPath: runscPath,
		RootDir:   rootDir,
	}
}

// Launch implements SandboxRuntime interface.
// NOTE: This is a stub implementation for now. Full gVisor integration
// would require implementing OCI spec conversion and runsc integration.
func (g *GVisorRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	g.Logger.Info("Launching gVisor sandbox (stub)", "id", req.ID)

	// TODO: Implement actual gVisor container creation using runsc
	// This would involve:
	// 1. Converting SandboxRequest to OCI spec
	// 2. Creating container bundle
	// 3. Calling runsc create
	// 4. Calling runsc start

	container := &gvisorContainer{
		ID:        req.ID,
		Request:   req,
		Config:    cfg,
		StartedAt: time.Now(),
	}

	g.containers.Store(req.ID, container)

	return &domain.SandboxRun{
		ID:        req.ID,
		RequestID: req.ID,
		NodeID:    req.NodeID,
		Template:  req.Template,
		Status:    domain.RunStatusRunning,
		StartedAt: container.StartedAt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  req.Metadata,
	}, nil
}

// Inspect implements SandboxRuntime interface.
func (g *GVisorRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	val, ok := g.containers.Load(id)
	if !ok {
		return nil, fmt.Errorf("container not found: %s", id)
	}

	container := val.(*gvisorContainer)
	container.mu.Lock()
	defer container.mu.Unlock()

	status := domain.RunStatusRunning
	if container.ExitCode != nil {
		if *container.ExitCode == 0 {
			status = domain.RunStatusSucceeded
		} else {
			status = domain.RunStatusFailed
		}
	}

	return &domain.SandboxRun{
		ID:        container.ID,
		RequestID: container.Request.ID,
		NodeID:    container.Request.NodeID,
		Template:  container.Request.Template,
		Status:    status,
		ExitCode:  container.ExitCode,
		StartedAt: container.StartedAt,
		CreatedAt: container.StartedAt,
		UpdatedAt: time.Now(),
		Metadata:  container.Request.Metadata,
	}, nil
}

// List implements SandboxRuntime interface.
func (g *GVisorRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var runs []domain.SandboxRun

	g.containers.Range(func(key, value interface{}) bool {
		container := value.(*gvisorContainer)
		container.mu.Lock()

		status := domain.RunStatusRunning
		if container.ExitCode != nil {
			if *container.ExitCode == 0 {
				status = domain.RunStatusSucceeded
			} else {
				status = domain.RunStatusFailed
			}
		}

		runs = append(runs, domain.SandboxRun{
			ID:        container.ID,
			RequestID: container.Request.ID,
			NodeID:    container.Request.NodeID,
			Template:  container.Request.Template,
			Status:    status,
			ExitCode:  container.ExitCode,
			StartedAt: container.StartedAt,
			CreatedAt: container.StartedAt,
			UpdatedAt: time.Now(),
			Metadata:  container.Request.Metadata,
		})

		container.mu.Unlock()
		return true
	})

	return runs, nil
}

// Kill implements SandboxRuntime interface.
func (g *GVisorRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}

	// TODO: Call runsc kill

	container := val.(*gvisorContainer)
	container.mu.Lock()
	exitCode := 137 // SIGKILL
	container.ExitCode = &exitCode
	container.mu.Unlock()

	g.containers.Delete(id)
	g.Logger.Info("gVisor container killed", "id", id)
	return nil
}

// Pause implements SandboxRuntime interface.
func (g *GVisorRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	// TODO: Call runsc pause
	g.Logger.Info("gVisor container paused (stub)", "id", id)
	return nil
}

// Resume implements SandboxRuntime interface.
func (g *GVisorRuntime) Resume(ctx context.Context, id domain.SandboxID) error {
	// TODO: Call runsc resume
	g.Logger.Info("gVisor container resumed (stub)", "id", id)
	return nil
}

// CreateSnapshot implements SandboxRuntime interface.
// gVisor supports checkpoint/restore natively.
func (g *GVisorRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	// TODO: Call runsc checkpoint
	g.Logger.Info("gVisor checkpoint created (stub)", "id", id)
	return nil
}

// Shutdown implements SandboxRuntime interface.
func (g *GVisorRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	// gVisor doesn't have a graceful shutdown like Firecracker's CtrlAltDel
	// Just kill the container
	return g.Kill(ctx, id)
}

// GetConfig implements SandboxRuntime interface.
func (g *GVisorRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (VMConfig, *domain.SandboxRequest, error) {
	val, ok := g.containers.Load(id)
	if !ok {
		return VMConfig{}, nil, fmt.Errorf("container not found: %s", id)
	}

	container := val.(*gvisorContainer)
	return container.Config, container.Request, nil
}

// StreamLogs implements SandboxRuntime interface.
func (g *GVisorRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	// TODO: Implement log streaming from gVisor container
	return fmt.Errorf("log streaming not implemented for gVisor runtime")
}

// Allocation implements SandboxRuntime interface.
func (g *GVisorRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	// Estimate: gVisor has moderate overhead
	count := 0
	g.containers.Range(func(_, _ interface{}) bool {
		count++
		return true
	})

	return domain.ResourceCapacity{
		CPU: domain.MilliCPU(count * 200), // 0.2 CPU per container
		Mem: domain.Megabytes(count * 50), // 50MB per container
		GPU: 0,
	}, nil
}

// Wait implements SandboxRuntime interface.
func (g *GVisorRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	// TODO: Call runsc wait or poll state
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}

	container := val.(*gvisorContainer)

	// Poll for completion
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			container.mu.Lock()
			finished := container.ExitCode != nil
			container.mu.Unlock()

			if finished {
				return nil
			}
		}
	}
}

// Exec implements SandboxRuntime interface.
func (g *GVisorRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string) error {
	// TODO: Call runsc exec
	g.Logger.Info("gVisor exec (stub)", "id", id, "cmd", cmd)
	return nil
}
