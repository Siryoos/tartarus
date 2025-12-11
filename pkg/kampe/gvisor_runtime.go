package kampe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// gvisorState tracks a running gVisor sandbox's state
type gvisorState struct {
	SandboxID   string
	BundlePath  string
	Request     *domain.SandboxRequest
	Config      tartarus.VMConfig
	StartedAt   time.Time
	ExitCode    *int
	Cmd         *exec.Cmd
	ConsoleFile *os.File
	mu          sync.Mutex
}

// GVisorAdapter wraps gVisor (runsc) with full SandboxRuntime implementation
type GVisorAdapter struct {
	runscPath  string
	bundleRoot string
	platform   string   // "ptrace" or "kvm"
	containers sync.Map // SandboxID -> *gvisorState
}

// NewGVisorAdapter creates a new gVisor adapter
func NewGVisorAdapter(socketPath string) (*GVisorAdapter, error) {
	// socketPath is reused as runsc binary path for gVisor
	runscPath := socketPath
	if runscPath == "" {
		runscPath = "/usr/local/bin/runsc"
	}

	// Check if runsc exists
	if _, err := exec.LookPath(runscPath); err != nil {
		// Try to find in PATH
		if path, pathErr := exec.LookPath("runsc"); pathErr == nil {
			runscPath = path
		} else {
			return nil, fmt.Errorf("runsc not found: %w", err)
		}
	}

	// Create bundle root directory
	bundleRoot := "/var/lib/tartarus/gvisor"
	if err := os.MkdirAll(bundleRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bundle root: %w", err)
	}

	// Detect platform capability
	platform := "ptrace"
	if _, err := os.Stat("/dev/kvm"); err == nil {
		platform = "kvm"
	}

	return &GVisorAdapter{
		runscPath:  runscPath,
		bundleRoot: bundleRoot,
		platform:   platform,
	}, nil
}

// createOCISpec creates an OCI runtime spec for the sandbox
func (g *GVisorAdapter) createOCISpec(req *domain.SandboxRequest, cfg tartarus.VMConfig) *specs.Spec {
	// Base spec
	spec := &specs.Spec{
		Version: "1.0.2",
		Root: &specs.Root{
			Path:     "rootfs",
			Readonly: false,
		},
		Process: &specs.Process{
			Terminal: false,
			User: specs.User{
				UID: 0,
				GID: 0,
			},
			Args: append(req.Command, req.Args...),
			Cwd:  "/",
		},
		Linux: &specs.Linux{
			Namespaces: []specs.LinuxNamespace{
				{Type: specs.PIDNamespace},
				{Type: specs.NetworkNamespace},
				{Type: specs.IPCNamespace},
				{Type: specs.UTSNamespace},
				{Type: specs.MountNamespace},
			},
			Resources: &specs.LinuxResources{},
		},
	}

	// Set environment variables
	spec.Process.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	for k, v := range req.Env {
		spec.Process.Env = append(spec.Process.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set resource limits
	if req.Resources.Mem > 0 {
		memLimit := int64(req.Resources.Mem) * 1024 * 1024
		spec.Linux.Resources.Memory = &specs.LinuxMemory{
			Limit: &memLimit,
		}
	}
	if req.Resources.CPU > 0 {
		quota := int64(req.Resources.CPU) * 100 // MilliCPU to quota
		period := uint64(100000)
		spec.Linux.Resources.CPU = &specs.LinuxCPU{
			Quota:  &quota,
			Period: &period,
		}
	}

	return spec
}

// getState retrieves the state for a sandbox
func (g *GVisorAdapter) getState(id domain.SandboxID) (*gvisorState, error) {
	val, ok := g.containers.Load(id)
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	return val.(*gvisorState), nil
}

// Launch creates and starts a gVisor sandbox
func (g *GVisorAdapter) Launch(ctx context.Context, req *domain.SandboxRequest, cfg tartarus.VMConfig) (*domain.SandboxRun, error) {
	sandboxID := string(req.ID)
	bundlePath := filepath.Join(g.bundleRoot, sandboxID)

	// Create bundle directory
	if err := os.MkdirAll(bundlePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bundle dir: %w", err)
	}

	// Create rootfs directory (should be populated with actual rootfs in production)
	rootfsPath := filepath.Join(bundlePath, "rootfs")
	if err := os.MkdirAll(rootfsPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create rootfs dir: %w", err)
	}

	// If overlay is specified, use it as rootfs
	if cfg.OverlayFS != "" {
		rootfsPath = cfg.OverlayFS
	}

	// Create OCI spec
	spec := g.createOCISpec(req, cfg)
	spec.Root.Path = rootfsPath

	// Write config.json
	configPath := filepath.Join(bundlePath, "config.json")
	configData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %w", err)
	}
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write config: %w", err)
	}

	// Create console log file
	consolePath := filepath.Join(bundlePath, "console.log")
	consoleFile, err := os.Create(consolePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create console file: %w", err)
	}

	// Build runsc command
	args := []string{
		"--platform=" + g.platform,
		"--rootless=false",
		"--network=sandbox",
		"run",
		"--bundle", bundlePath,
		sandboxID,
	}

	cmd := exec.CommandContext(ctx, g.runscPath, args...)
	cmd.Stdout = consoleFile
	cmd.Stderr = consoleFile
	cmd.Dir = bundlePath

	// Start the sandbox
	if err := cmd.Start(); err != nil {
		consoleFile.Close()
		os.RemoveAll(bundlePath)
		return nil, fmt.Errorf("failed to start runsc: %w", err)
	}

	// Store state
	state := &gvisorState{
		SandboxID:   sandboxID,
		BundlePath:  bundlePath,
		Request:     req,
		Config:      cfg,
		StartedAt:   time.Now(),
		Cmd:         cmd,
		ConsoleFile: consoleFile,
	}
	g.containers.Store(req.ID, state)

	// Start background goroutine to capture exit status
	go func() {
		err := cmd.Wait()
		state.mu.Lock()
		defer state.mu.Unlock()
		consoleFile.Close()

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				state.ExitCode = &code
			} else {
				code := -1
				state.ExitCode = &code
			}
		} else {
			code := 0
			state.ExitCode = &code
		}
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
func (g *GVisorAdapter) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	state, err := g.getState(id)
	if err != nil {
		return nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	status := domain.RunStatusRunning
	var exitCode *int
	var finishedAt time.Time

	if state.ExitCode != nil {
		exitCode = state.ExitCode
		if *exitCode == 0 {
			status = domain.RunStatusSucceeded
		} else {
			status = domain.RunStatusFailed
		}
		finishedAt = time.Now()
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
func (g *GVisorAdapter) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var runs []domain.SandboxRun

	g.containers.Range(func(key, value any) bool {
		id := key.(domain.SandboxID)
		run, err := g.Inspect(ctx, id)
		if err == nil {
			runs = append(runs, *run)
		}
		return true
	})

	return runs, nil
}

// Kill terminates a sandbox forcefully
func (g *GVisorAdapter) Kill(ctx context.Context, id domain.SandboxID) error {
	state, err := g.getState(id)
	if err != nil {
		return nil // Already gone
	}

	// Kill using runsc
	killCmd := exec.CommandContext(ctx, g.runscPath, "kill", state.SandboxID, "KILL")
	if err := killCmd.Run(); err != nil {
		// Try to kill the process directly
		if state.Cmd != nil && state.Cmd.Process != nil {
			_ = state.Cmd.Process.Kill()
		}
	}

	// Delete the sandbox
	deleteCmd := exec.CommandContext(ctx, g.runscPath, "delete", "--force", state.SandboxID)
	_ = deleteCmd.Run()

	// Cleanup bundle
	os.RemoveAll(state.BundlePath)

	g.containers.Delete(id)
	return nil
}

// Pause pauses a running sandbox
func (g *GVisorAdapter) Pause(ctx context.Context, id domain.SandboxID) error {
	state, err := g.getState(id)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, g.runscPath, "pause", state.SandboxID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pause sandbox: %w", err)
	}

	return nil
}

// Resume unpauses a paused sandbox
func (g *GVisorAdapter) Resume(ctx context.Context, id domain.SandboxID) error {
	state, err := g.getState(id)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, g.runscPath, "resume", state.SandboxID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to resume sandbox: %w", err)
	}

	return nil
}

// CreateSnapshot creates a checkpoint of the sandbox (gVisor supports this)
func (g *GVisorAdapter) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	state, err := g.getState(id)
	if err != nil {
		return err
	}

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(memPath), 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// gVisor checkpoint command
	cmd := exec.CommandContext(ctx, g.runscPath,
		"checkpoint",
		"--image-path", filepath.Dir(memPath),
		state.SandboxID)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkpoint sandbox: %w", err)
	}

	return nil
}

// Shutdown gracefully stops the sandbox
func (g *GVisorAdapter) Shutdown(ctx context.Context, id domain.SandboxID) error {
	state, err := g.getState(id)
	if err != nil {
		return err
	}

	// Send SIGTERM
	killCmd := exec.CommandContext(ctx, g.runscPath, "kill", state.SandboxID, "TERM")
	if err := killCmd.Run(); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for graceful shutdown with timeout
	done := make(chan struct{})
	go func() {
		if state.Cmd != nil {
			_ = state.Cmd.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(30 * time.Second):
		// Force kill
		return g.Kill(ctx, id)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetConfig returns the VM config and original request
func (g *GVisorAdapter) GetConfig(ctx context.Context, id domain.SandboxID) (tartarus.VMConfig, *domain.SandboxRequest, error) {
	state, err := g.getState(id)
	if err != nil {
		return tartarus.VMConfig{}, nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	return state.Config, state.Request, nil
}

// StreamLogs streams sandbox logs
func (g *GVisorAdapter) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	state, err := g.getState(id)
	if err != nil {
		return err
	}

	consolePath := filepath.Join(state.BundlePath, "console.log")
	file, err := os.Open(consolePath)
	if err != nil {
		return fmt.Errorf("failed to open console log: %w", err)
	}
	defer file.Close()

	if !follow {
		_, err = io.Copy(w, file)
		return err
	}

	// Follow mode
	buf := make([]byte, 1024)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := file.Read(buf)
			if n > 0 {
				if _, wErr := w.Write(buf[:n]); wErr != nil {
					return wErr
				}
			}
			if err == io.EOF {
				state.mu.Lock()
				done := state.ExitCode != nil
				state.mu.Unlock()
				if done {
					return nil
				}
				continue
			}
			if err != nil {
				return err
			}
		}
	}
}

// Allocation returns the total resources allocated
func (g *GVisorAdapter) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	var cpu domain.MilliCPU
	var mem domain.Megabytes

	g.containers.Range(func(key, value any) bool {
		state := value.(*gvisorState)
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
func (g *GVisorAdapter) Wait(ctx context.Context, id domain.SandboxID) error {
	state, err := g.getState(id)
	if err != nil {
		return err
	}

	// Poll for exit status
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			state.mu.Lock()
			done := state.ExitCode != nil
			state.mu.Unlock()
			if done {
				return nil
			}
		}
	}
}

// Exec executes a command in the sandbox
func (g *GVisorAdapter) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	state, err := g.getState(id)
	if err != nil {
		return err
	}

	// Build exec command
	args := append([]string{"exec", state.SandboxID, "--"}, cmd...)
	execCmd := exec.CommandContext(ctx, g.runscPath, args...)
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to exec: %w", err)
	}

	return nil
}

// ExecInteractive executes an interactive command
func (g *GVisorAdapter) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	state, err := g.getState(id)
	if err != nil {
		return err
	}

	// Build exec command with terminal
	args := append([]string{"exec", "--console-socket", "-", state.SandboxID, "--"}, cmd...)
	execCmd := exec.CommandContext(ctx, g.runscPath, args...)
	execCmd.Stdin = stdin
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to exec: %w", err)
	}

	return nil
}

// Migration helpers

// CanMigrate checks if a sandbox can be migrated to microVM
func (g *GVisorAdapter) CanMigrate(ctx context.Context, containerID string) (bool, error) {
	// gVisor sandboxes are generally well-suited for migration
	return true, nil
}

// MigrateToMicroVM creates a migration plan
func (g *GVisorAdapter) MigrateToMicroVM(ctx context.Context, containerID string) (*MigrationPlan, error) {
	plan := &MigrationPlan{
		ContainerID:       containerID,
		TargetTemplate:    "microvm-gvisor-compatible",
		RiskLevel:         RiskLevelLow,
		EstimatedDowntime: 5 * time.Second,
		Recommendations: []string{
			"gVisor workloads are already sandboxed, migration is low risk",
			"Verify syscall compatibility between gVisor and microVM",
		},
	}

	return plan, nil
}

// ExportState exports sandbox state
func (g *GVisorAdapter) ExportState(ctx context.Context, containerID string) (*ContainerState, error) {
	// Check if we have this container tracked
	var state *gvisorState
	g.containers.Range(func(key, value any) bool {
		s := value.(*gvisorState)
		if s.SandboxID == containerID {
			state = s
			return false
		}
		return true
	})

	if state == nil {
		// Try to read from bundle
		bundlePath := filepath.Join(g.bundleRoot, containerID)
		configPath := filepath.Join(bundlePath, "config.json")

		configData, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("sandbox not found: %s", containerID)
		}

		var spec specs.Spec
		if err := json.Unmarshal(configData, &spec); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}

		result := &ContainerState{
			ID:    containerID,
			Image: "gvisor-sandbox",
			Config: ContainerConfig{
				WorkingDir: spec.Process.Cwd,
				Env:        spec.Process.Env,
			},
			Environment: make(map[string]string),
		}

		if len(spec.Process.Args) > 0 {
			result.Config.Entrypoint = spec.Process.Args[:1]
			result.Config.Cmd = spec.Process.Args[1:]
		}

		for _, e := range spec.Process.Env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				result.Environment[parts[0]] = parts[1]
			}
		}

		return result, nil
	}

	return &ContainerState{
		ID:    containerID,
		Image: string(state.Request.Template),
		Config: ContainerConfig{
			Cmd:        state.Request.Command,
			WorkingDir: "/",
		},
		Environment: state.Request.Env,
	}, nil
}

// Ensure GVisorAdapter implements LegacyRuntime
var _ LegacyRuntime = (*GVisorAdapter)(nil)

// Unused import prevention
var _ = syscall.SIGTERM
