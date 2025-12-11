package kampe

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

const (
	// defaultNamespace is the containerd namespace for Tartarus sandboxes
	defaultNamespace = "tartarus"
)

// containerdState tracks a running containerd container's state
type containerdState struct {
	Container   containerd.Container
	Task        containerd.Task
	Request     *domain.SandboxRequest
	Config      tartarus.VMConfig
	StartedAt   time.Time
	ExitCode    *int
	ExitChannel <-chan containerd.ExitStatus
	mu          sync.Mutex
}

// ContainerdAdapter wraps containerd with full SandboxRuntime implementation
type ContainerdAdapter struct {
	client     *containerd.Client
	socketPath string
	namespace  string
	containers sync.Map // SandboxID -> *containerdState
}

// NewContainerdAdapter creates a new containerd adapter
func NewContainerdAdapter(socketPath string) (*ContainerdAdapter, error) {
	if socketPath == "" {
		socketPath = "/run/containerd/containerd.sock"
	}

	client, err := containerd.New(socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ctx = namespaces.WithNamespace(ctx, defaultNamespace)
	if _, err := client.Version(ctx); err != nil {
		return nil, fmt.Errorf("failed to get containerd version: %w", err)
	}

	return &ContainerdAdapter{
		client:     client,
		socketPath: socketPath,
		namespace:  defaultNamespace,
	}, nil
}

// withNamespace adds the Tartarus namespace to the context
func (c *ContainerdAdapter) withNamespace(ctx context.Context) context.Context {
	return namespaces.WithNamespace(ctx, c.namespace)
}

// getState retrieves the state for a sandbox
func (c *ContainerdAdapter) getState(id domain.SandboxID) (*containerdState, error) {
	val, ok := c.containers.Load(id)
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	return val.(*containerdState), nil
}

// Launch creates and starts a containerd container
func (c *ContainerdAdapter) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	ctx = c.withNamespace(ctx)

	// Pull image if needed
	image, err := c.client.GetImage(ctx, string(req.Template))
	if err != nil {
		// Try to pull the image
		image, err = c.client.Pull(ctx, string(req.Template), containerd.WithPullUnpack)
		if err != nil {
			return nil, fmt.Errorf("failed to get or pull image %s: %w", req.Template, err)
		}
	}

	// Build OCI spec
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithProcessArgs(append(req.Command, req.Args...)...),
	}

	// Add environment variables
	if len(req.Env) > 0 {
		var envs []string
		for k, v := range req.Env {
			envs = append(envs, fmt.Sprintf("%s=%s", k, v))
		}
		specOpts = append(specOpts, oci.WithEnv(envs))
	}

	// Add resource limits
	if req.Resources.Mem > 0 {
		memLimit := int64(req.Resources.Mem) * 1024 * 1024 // MB to bytes
		specOpts = append(specOpts, oci.WithMemoryLimit(uint64(memLimit)))
	}
	if req.Resources.CPU > 0 {
		// CPU quota in microseconds per 100ms period
		quota := int64(req.Resources.CPU) * 100 // MilliCPU * 100us = quota
		specOpts = append(specOpts, oci.WithCPUCFS(quota, 100000))
	}

	// Create container
	containerID := string(req.ID)
	container, err := c.client.NewContainer(
		ctx,
		containerID,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(containerID+"-snapshot", image),
		containerd.WithNewSpec(specOpts...),
		containerd.WithContainerLabels(map[string]string{
			"tartarus.sandbox.id": containerID,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Create task (the running process)
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Setup exit channel before starting
	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		task.Delete(ctx)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to wait on task: %w", err)
	}

	// Start the task
	if err := task.Start(ctx); err != nil {
		task.Delete(ctx)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	// Store state
	state := &containerdState{
		Container:   container,
		Task:        task,
		Request:     req,
		Config:      cfg,
		StartedAt:   time.Now(),
		ExitChannel: exitStatusC,
	}
	c.containers.Store(req.ID, state)

	// Start background goroutine to capture exit status
	go func() {
		exitStatus := <-exitStatusC
		state.mu.Lock()
		code := int(exitStatus.ExitCode())
		state.ExitCode = &code
		state.mu.Unlock()
	}()

	return &domain.SandboxRun{
		ID:        req.ID,
		RequestID: req.ID,
		Status:    domain.RunStatusRunning,
		StartedAt: state.StartedAt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// Inspect returns the current state of a sandbox
func (c *ContainerdAdapter) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	ctx = c.withNamespace(ctx)

	state, err := c.getState(id)
	if err != nil {
		return nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	status := domain.RunStatusRunning
	var exitCode *int
	var finishedAt time.Time

	// Check task status
	taskStatus, err := state.Task.Status(ctx)
	if err == nil {
		switch taskStatus.Status {
		case containerd.Running:
			status = domain.RunStatusRunning
		case containerd.Stopped:
			if state.ExitCode != nil {
				exitCode = state.ExitCode
				if *exitCode == 0 {
					status = domain.RunStatusSucceeded
				} else {
					status = domain.RunStatusFailed
				}
				finishedAt = taskStatus.ExitTime
			}
		case containerd.Paused:
			status = domain.RunStatusRunning // Still considered running but paused
		}
	}

	return &domain.SandboxRun{
		ID:         id,
		RequestID:  state.Request.ID,
		Status:     status,
		ExitCode:   exitCode,
		StartedAt:  state.StartedAt,
		FinishedAt: finishedAt,
		UpdatedAt:  time.Now(),
	}, nil
}

// List returns all active sandboxes
func (c *ContainerdAdapter) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var runs []domain.SandboxRun

	c.containers.Range(func(key, value any) bool {
		id := key.(domain.SandboxID)
		run, err := c.Inspect(ctx, id)
		if err == nil {
			runs = append(runs, *run)
		}
		return true
	})

	return runs, nil
}

// Kill terminates a sandbox forcefully
func (c *ContainerdAdapter) Kill(ctx context.Context, id domain.SandboxID) error {
	ctx = c.withNamespace(ctx)

	state, err := c.getState(id)
	if err != nil {
		return nil // Already gone
	}

	// Kill the task
	if err := state.Task.Kill(ctx, syscall.SIGKILL); err != nil {
		// Ignore errors, task might already be dead
	}

	// Delete the task
	if _, err := state.Task.Delete(ctx); err != nil {
		// Ignore errors
	}

	// Delete the container and snapshot
	if err := state.Container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}

	c.containers.Delete(id)
	return nil
}

// Pause pauses a running sandbox
func (c *ContainerdAdapter) Pause(ctx context.Context, id domain.SandboxID) error {
	ctx = c.withNamespace(ctx)

	state, err := c.getState(id)
	if err != nil {
		return err
	}

	if err := state.Task.Pause(ctx); err != nil {
		return fmt.Errorf("failed to pause task: %w", err)
	}

	return nil
}

// Resume unpauses a paused sandbox
func (c *ContainerdAdapter) Resume(ctx context.Context, id domain.SandboxID) error {
	ctx = c.withNamespace(ctx)

	state, err := c.getState(id)
	if err != nil {
		return err
	}

	if err := state.Task.Resume(ctx); err != nil {
		return fmt.Errorf("failed to resume task: %w", err)
	}

	return nil
}

// CreateSnapshot creates a checkpoint of the container
func (c *ContainerdAdapter) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	ctx = c.withNamespace(ctx)

	state, err := c.getState(id)
	if err != nil {
		return err
	}

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(memPath), 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Checkpoint the task
	checkpointDir := filepath.Dir(memPath)
	_, err = state.Task.Checkpoint(ctx, containerd.WithCheckpointImagePath(checkpointDir))
	if err != nil {
		return fmt.Errorf("failed to checkpoint task: %w", err)
	}

	return nil
}

// Shutdown gracefully stops the sandbox
func (c *ContainerdAdapter) Shutdown(ctx context.Context, id domain.SandboxID) error {
	ctx = c.withNamespace(ctx)

	state, err := c.getState(id)
	if err != nil {
		return err
	}

	// Send SIGTERM for graceful shutdown
	if err := state.Task.Kill(ctx, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for task to exit with timeout
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	select {
	case <-state.ExitChannel:
		return nil
	case <-waitCtx.Done():
		// Timeout, force kill
		return state.Task.Kill(ctx, syscall.SIGKILL)
	}
}

// GetConfig returns the VM config and original request
func (c *ContainerdAdapter) GetConfig(ctx context.Context, id domain.SandboxID) (tartarus.VMConfig, *domain.SandboxRequest, error) {
	state, err := c.getState(id)
	if err != nil {
		return tartarus.VMConfig{}, nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	return state.Config, state.Request, nil
}

// StreamLogs streams container logs
func (c *ContainerdAdapter) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	// containerd doesn't have built-in log storage like Docker
	// Logs would need to be captured from the task's stdio
	return fmt.Errorf("log streaming requires custom log driver configuration")
}

// Allocation returns the total resources allocated to running containers
func (c *ContainerdAdapter) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	var cpu domain.MilliCPU
	var mem domain.Megabytes

	c.containers.Range(func(key, value any) bool {
		state := value.(*containerdState)
		state.mu.Lock()
		if state.ExitCode == nil {
			cpu += state.Request.Resources.CPU
			mem += state.Request.Resources.Mem
		}
		state.mu.Unlock()
		return true
	})

	return domain.ResourceCapacity{
		CPU: cpu,
		Mem: mem,
		GPU: 0,
	}, nil
}

// Wait blocks until the sandbox exits
func (c *ContainerdAdapter) Wait(ctx context.Context, id domain.SandboxID) error {
	state, err := c.getState(id)
	if err != nil {
		return err
	}

	// Wait on the exit channel
	select {
	case exitStatus := <-state.ExitChannel:
		state.mu.Lock()
		code := int(exitStatus.ExitCode())
		state.ExitCode = &code
		state.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Exec executes a command in the sandbox
func (c *ContainerdAdapter) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	ctx = c.withNamespace(ctx)

	state, err := c.getState(id)
	if err != nil {
		return err
	}

	// Create exec process spec
	pspec := &specs.Process{
		Args: cmd,
		Cwd:  "/",
	}

	// Create IO
	ioCreator := cio.NewCreator(cio.WithStreams(nil, stdout, stderr))

	// Execute
	process, err := state.Task.Exec(ctx, fmt.Sprintf("exec-%d", time.Now().UnixNano()), pspec, ioCreator)
	if err != nil {
		return fmt.Errorf("failed to exec: %w", err)
	}
	defer process.Delete(ctx)

	// Start the exec process
	if err := process.Start(ctx); err != nil {
		return fmt.Errorf("failed to start exec: %w", err)
	}

	// Wait for completion
	exitStatus, err := process.Wait(ctx)
	if err != nil {
		return err
	}

	<-exitStatus
	return nil
}

// ExecInteractive executes an interactive command
func (c *ContainerdAdapter) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	ctx = c.withNamespace(ctx)

	state, err := c.getState(id)
	if err != nil {
		return err
	}

	// Create exec process spec with terminal
	pspec := &specs.Process{
		Args:     cmd,
		Cwd:      "/",
		Terminal: true,
	}

	// Create IO with all streams
	ioCreator := cio.NewCreator(cio.WithStreams(stdin, stdout, stderr), cio.WithTerminal)

	// Execute
	process, err := state.Task.Exec(ctx, fmt.Sprintf("exec-interactive-%d", time.Now().UnixNano()), pspec, ioCreator)
	if err != nil {
		return fmt.Errorf("failed to exec: %w", err)
	}
	defer process.Delete(ctx)

	// Start the exec process
	if err := process.Start(ctx); err != nil {
		return fmt.Errorf("failed to start exec: %w", err)
	}

	// Wait for completion
	exitStatus, err := process.Wait(ctx)
	if err != nil {
		return err
	}

	<-exitStatus
	return nil
}

// Migration helpers

// CanMigrate checks if a container can be migrated to microVM
func (c *ContainerdAdapter) CanMigrate(ctx context.Context, containerID string) (bool, error) {
	ctx = c.withNamespace(ctx)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return false, fmt.Errorf("failed to load container: %w", err)
	}

	spec, err := container.Spec(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get spec: %w", err)
	}

	// Check for features that are hard to migrate
	if spec.Linux != nil && spec.Linux.Namespaces != nil {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Type == specs.NetworkNamespace && ns.Path != "" {
				// Using host network namespace
				return false, nil
			}
		}
	}

	return true, nil
}

// MigrateToMicroVM creates a migration plan
func (c *ContainerdAdapter) MigrateToMicroVM(ctx context.Context, containerID string) (*MigrationPlan, error) {
	ctx = c.withNamespace(ctx)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	info, err := container.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}

	plan := &MigrationPlan{
		ContainerID:       containerID,
		TargetTemplate:    "microvm-containerd-compatible",
		RiskLevel:         RiskLevelLow,
		EstimatedDowntime: 10 * time.Second,
		Recommendations: []string{
			"Test the migrated workload in staging",
			"Verify environment configuration",
		},
	}

	// Check labels for hints
	if info.Labels["io.containerd.runc.v2.group"] != "" {
		plan.Recommendations = append(plan.Recommendations,
			"Container uses cgroup settings that may need adjustment")
	}

	return plan, nil
}

// ExportState exports container state
func (c *ContainerdAdapter) ExportState(ctx context.Context, containerID string) (*ContainerState, error) {
	ctx = c.withNamespace(ctx)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	info, err := container.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}

	spec, err := container.Spec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get spec: %w", err)
	}

	state := &ContainerState{
		ID:    containerID,
		Image: info.Image,
		Config: ContainerConfig{
			WorkingDir: spec.Process.Cwd,
			User:       spec.Process.User.Username,
			Env:        spec.Process.Env,
		},
		Environment: make(map[string]string),
	}

	// Parse environment
	for _, e := range spec.Process.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			state.Environment[parts[0]] = parts[1]
		}
	}

	if spec.Process.Args != nil {
		if len(spec.Process.Args) > 0 {
			state.Config.Entrypoint = spec.Process.Args[:1]
			state.Config.Cmd = spec.Process.Args[1:]
		}
	}

	return state, nil
}

// Ensure ContainerdAdapter implements LegacyRuntime
var _ LegacyRuntime = (*ContainerdAdapter)(nil)

// Unused import prevention
var _ containers.Container
