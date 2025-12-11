package kampe

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/checkpoint"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// dockerState tracks a running Docker container's state
type dockerState struct {
	ContainerID string
	Request     *domain.SandboxRequest
	Config      tartarus.VMConfig
	StartedAt   time.Time
	ExitCode    *int
	mu          sync.Mutex
}

// DockerAdapter wraps Docker Engine with full SandboxRuntime implementation
type DockerAdapter struct {
	client     *client.Client
	socketPath string
	containers sync.Map // SandboxID -> *dockerState
}

// NewDockerAdapter creates a new Docker adapter connected to the specified socket
func NewDockerAdapter(socketPath string) (*DockerAdapter, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if socketPath != "" {
		opts = append(opts, client.WithHost("unix://"+socketPath))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to docker: %w", err)
	}

	return &DockerAdapter{
		client:     cli,
		socketPath: socketPath,
	}, nil
}

// getState retrieves the state for a sandbox
func (d *DockerAdapter) getState(id domain.SandboxID) (*dockerState, error) {
	val, ok := d.containers.Load(id)
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	return val.(*dockerState), nil
}

// Launch creates and starts a Docker container
func (d *DockerAdapter) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	// Build environment variables
	var env []string
	for k, v := range req.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Build container config
	containerCfg := &container.Config{
		Image:      string(req.Template), // Use template as image name
		Cmd:        append(req.Command, req.Args...),
		Env:        env,
		WorkingDir: "/",
		Labels: map[string]string{
			"tartarus.sandbox.id": string(req.ID),
		},
	}

	// Build host config with resource limits
	hostCfg := &container.HostConfig{
		Resources: container.Resources{
			Memory:   int64(req.Resources.Mem) * 1024 * 1024, // MB to bytes
			NanoCPUs: int64(req.Resources.CPU) * 1000000,     // MilliCPU to NanoCPUs
		},
		AutoRemove: false,
	}

	// Create volume mounts if overlay is specified
	if cfg.OverlayFS != "" {
		hostCfg.Binds = append(hostCfg.Binds, fmt.Sprintf("%s:/overlay:rw", cfg.OverlayFS))
	}

	// Ensure image exists
	if err := d.ensureImage(ctx, string(req.Template)); err != nil {
		return nil, fmt.Errorf("failed to ensure image: %w", err)
	}

	// Create the container
	containerName := fmt.Sprintf("tartarus-%s", req.ID)
	resp, err := d.client.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Start the container
	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Clean up on failure
		_ = d.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Store state
	state := &dockerState{
		ContainerID: resp.ID,
		Request:     req,
		Config:      cfg,
		StartedAt:   time.Now(),
	}
	d.containers.Store(req.ID, state)

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
func (d *DockerAdapter) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	state, err := d.getState(id)
	if err != nil {
		return nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	// Get container info from Docker
	info, err := d.client.ContainerInspect(ctx, state.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	status := domain.RunStatusRunning
	var exitCode *int
	var finishedAt time.Time

	if !info.State.Running {
		code := info.State.ExitCode
		exitCode = &code
		state.ExitCode = exitCode
		if code == 0 {
			status = domain.RunStatusSucceeded
		} else {
			status = domain.RunStatusFailed
		}
		if info.State.FinishedAt != "" {
			finishedAt, _ = time.Parse(time.RFC3339Nano, info.State.FinishedAt)
		}
	}

	// Calculate memory usage
	var memUsage domain.Megabytes
	stats, err := d.client.ContainerStats(ctx, state.ContainerID, false)
	if err == nil {
		defer stats.Body.Close()
		// Parse stats for memory usage (simplified)
		// In production, decode the JSON stats properly
	}

	return &domain.SandboxRun{
		ID:          id,
		RequestID:   state.Request.ID,
		Status:      status,
		ExitCode:    exitCode,
		StartedAt:   state.StartedAt,
		FinishedAt:  finishedAt,
		UpdatedAt:   time.Now(),
		MemoryUsage: memUsage,
	}, nil
}

// List returns all active sandboxes
func (d *DockerAdapter) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var runs []domain.SandboxRun

	d.containers.Range(func(key, value any) bool {
		id := key.(domain.SandboxID)
		run, err := d.Inspect(ctx, id)
		if err == nil {
			runs = append(runs, *run)
		}
		return true
	})

	return runs, nil
}

// Kill terminates a sandbox forcefully
func (d *DockerAdapter) Kill(ctx context.Context, id domain.SandboxID) error {
	state, err := d.getState(id)
	if err != nil {
		return nil // Already gone
	}

	// Stop the container
	timeout := 0
	if err := d.client.ContainerStop(ctx, state.ContainerID, container.StopOptions{Timeout: &timeout}); err != nil {
		// Ignore errors, try to remove anyway
	}

	// Remove the container
	if err := d.client.ContainerRemove(ctx, state.ContainerID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	d.containers.Delete(id)
	return nil
}

// Pause pauses a running sandbox
func (d *DockerAdapter) Pause(ctx context.Context, id domain.SandboxID) error {
	state, err := d.getState(id)
	if err != nil {
		return err
	}

	if err := d.client.ContainerPause(ctx, state.ContainerID); err != nil {
		return fmt.Errorf("failed to pause container: %w", err)
	}

	return nil
}

// Resume unpauses a paused sandbox
func (d *DockerAdapter) Resume(ctx context.Context, id domain.SandboxID) error {
	state, err := d.getState(id)
	if err != nil {
		return err
	}

	if err := d.client.ContainerUnpause(ctx, state.ContainerID); err != nil {
		return fmt.Errorf("failed to unpause container: %w", err)
	}

	return nil
}

// CreateSnapshot creates a checkpoint of the container (requires CRIU)
func (d *DockerAdapter) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	state, err := d.getState(id)
	if err != nil {
		return err
	}

	// Docker checkpoint requires CRIU installed
	checkpointOpts := checkpoint.CreateOptions{
		CheckpointID:  fmt.Sprintf("snapshot-%s-%d", id, time.Now().Unix()),
		CheckpointDir: memPath,
		Exit:          false, // Keep container running
	}

	if err := d.client.CheckpointCreate(ctx, state.ContainerID, checkpointOpts); err != nil {
		return fmt.Errorf("failed to create checkpoint (requires CRIU): %w", err)
	}

	return nil
}

// Shutdown gracefully stops the sandbox
func (d *DockerAdapter) Shutdown(ctx context.Context, id domain.SandboxID) error {
	state, err := d.getState(id)
	if err != nil {
		return err
	}

	timeout := 30 // 30 second graceful shutdown
	if err := d.client.ContainerStop(ctx, state.ContainerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return nil
}

// GetConfig returns the VM config and original request
func (d *DockerAdapter) GetConfig(ctx context.Context, id domain.SandboxID) (tartarus.VMConfig, *domain.SandboxRequest, error) {
	state, err := d.getState(id)
	if err != nil {
		return tartarus.VMConfig{}, nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	return state.Config, state.Request, nil
}

// StreamLogs streams container logs to the writer
func (d *DockerAdapter) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	state, err := d.getState(id)
	if err != nil {
		return err
	}

	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: false,
	}

	reader, err := d.client.ContainerLogs(ctx, state.ContainerID, opts)
	if err != nil {
		return fmt.Errorf("failed to get container logs: %w", err)
	}
	defer reader.Close()

	// Copy logs to writer (Docker logs have an 8-byte header per line)
	_, err = io.Copy(w, reader)
	return err
}

// Allocation returns the total resources allocated to running containers
func (d *DockerAdapter) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	var cpu domain.MilliCPU
	var mem domain.Megabytes

	d.containers.Range(func(key, value any) bool {
		state := value.(*dockerState)
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
func (d *DockerAdapter) Wait(ctx context.Context, id domain.SandboxID) error {
	state, err := d.getState(id)
	if err != nil {
		return err
	}

	statusCh, errCh := d.client.ContainerWait(ctx, state.ContainerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		return err
	case status := <-statusCh:
		state.mu.Lock()
		code := int(status.StatusCode)
		state.ExitCode = &code
		state.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Exec executes a command in the sandbox
func (d *DockerAdapter) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	state, err := d.getState(id)
	if err != nil {
		return err
	}

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := d.client.ContainerExecCreate(ctx, state.ContainerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec: %w", err)
	}

	resp, err := d.client.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return fmt.Errorf("failed to attach exec: %w", err)
	}
	defer resp.Close()

	// Copy output
	if stdout != nil || stderr != nil {
		if stdout == nil {
			stdout = io.Discard
		}
		if stderr == nil {
			stderr = io.Discard
		}
		// Docker multiplexes stdout/stderr, simplified handling here
		_, _ = io.Copy(stdout, resp.Reader)
	}

	return nil
}

// ExecInteractive executes an interactive command with stdin support
func (d *DockerAdapter) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	state, err := d.getState(id)
	if err != nil {
		return err
	}

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdin:  stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}

	execID, err := d.client.ContainerExecCreate(ctx, state.ContainerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec: %w", err)
	}

	resp, err := d.client.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		return fmt.Errorf("failed to attach exec: %w", err)
	}
	defer resp.Close()

	// Handle I/O
	var wg sync.WaitGroup

	if stdin != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = io.Copy(resp.Conn, stdin)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if stdout != nil {
			_, _ = io.Copy(stdout, resp.Reader)
		}
	}()

	wg.Wait()
	return nil
}

// Migration helpers

// CanMigrate checks if a container can be migrated to microVM
func (d *DockerAdapter) CanMigrate(ctx context.Context, containerID string) (bool, error) {
	info, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, fmt.Errorf("failed to inspect container: %w", err)
	}

	// Check for features that are hard to migrate
	if info.HostConfig.NetworkMode == "host" {
		return false, nil // Host network not supported in microVM
	}

	if len(info.HostConfig.Devices) > 0 {
		return false, nil // Direct device access not supported
	}

	return true, nil
}

// MigrateToMicroVM analyzes a container and creates a migration plan
func (d *DockerAdapter) MigrateToMicroVM(ctx context.Context, containerID string) (*MigrationPlan, error) {
	info, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	plan := &MigrationPlan{
		ContainerID:    containerID,
		TargetTemplate: "microvm-docker-compatible",
		RiskLevel:      RiskLevelLow,
	}

	// Analyze container configuration
	if info.HostConfig.NetworkMode == "host" {
		plan.RiskLevel = RiskLevelHigh
		plan.RequiredChanges = append(plan.RequiredChanges, MigrationChange{
			Type:        ChangeTypeNetwork,
			Description: "Host network mode not supported in microVM",
			Required:    true,
			AutoFix:     false,
		})
	}

	if len(info.HostConfig.Devices) > 0 {
		plan.RiskLevel = RiskLevelHigh
		plan.RequiredChanges = append(plan.RequiredChanges, MigrationChange{
			Type:        ChangeTypeResources,
			Description: fmt.Sprintf("%d device mounts not supported", len(info.HostConfig.Devices)),
			Required:    true,
			AutoFix:     false,
		})
	}

	if info.HostConfig.Privileged {
		plan.RiskLevel = RiskLevelMedium
		plan.RequiredChanges = append(plan.RequiredChanges, MigrationChange{
			Type:        ChangeTypeResources,
			Description: "Privileged mode will be disabled (microVM provides isolation)",
			Required:    false,
			AutoFix:     true,
		})
	}

	// Estimate downtime based on image size
	imageInfo, _, err := d.client.ImageInspectWithRaw(ctx, info.Image)
	if err == nil {
		sizeMB := imageInfo.Size / (1024 * 1024)
		plan.EstimatedDowntime = time.Duration(sizeMB/100) * time.Second // ~100MB/s conversion
		if plan.EstimatedDowntime < 5*time.Second {
			plan.EstimatedDowntime = 5 * time.Second
		}
	}

	plan.Recommendations = []string{
		"Test the migrated workload in staging before production",
		"Verify all environment variables are correctly passed",
	}

	if len(info.Mounts) > 0 {
		plan.Recommendations = append(plan.Recommendations,
			fmt.Sprintf("Review %d volume mounts for overlay filesystem compatibility", len(info.Mounts)))
	}

	return plan, nil
}

// ExportState exports the container state for migration
func (d *DockerAdapter) ExportState(ctx context.Context, containerID string) (*ContainerState, error) {
	info, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	state := &ContainerState{
		ID:    containerID,
		Image: info.Config.Image,
		Config: ContainerConfig{
			Entrypoint: info.Config.Entrypoint,
			Cmd:        info.Config.Cmd,
			WorkingDir: info.Config.WorkingDir,
			User:       info.Config.User,
			Env:        info.Config.Env,
		},
		Environment: make(map[string]string),
	}

	// Parse environment variables
	for _, e := range info.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			state.Environment[parts[0]] = parts[1]
		}
	}

	// Parse volumes
	for k := range info.Config.Volumes {
		state.Config.Volumes = append(state.Config.Volumes, k)
	}

	// Parse port mappings
	for port, bindings := range info.HostConfig.PortBindings {
		containerPort, _ := strconv.Atoi(port.Port())
		for _, binding := range bindings {
			hostPort, _ := strconv.Atoi(binding.HostPort)
			state.Config.Ports = append(state.Config.Ports, PortMapping{
				ContainerPort: containerPort,
				HostPort:      hostPort,
				Protocol:      port.Proto(),
			})
		}
	}

	return state, nil
}

func (d *DockerAdapter) ensureImage(ctx context.Context, imageName string) error {
	_, _, err := d.client.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil
	}

	if !client.IsErrNotFound(err) {
		return fmt.Errorf("failed to inspect image: %w", err)
	}

	// Image not found, pull it
	reader, err := d.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Drain output to wait for pull to completion
	_, err = io.Copy(io.Discard, reader)
	return err
}

// Ensure DockerAdapter implements LegacyRuntime
var _ LegacyRuntime = (*DockerAdapter)(nil)

// Unused import prevention
var _ = nat.Port("")
