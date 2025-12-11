package tartarus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"
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

	// Platform is the gVisor platform to use (e.g. "ptrace" or "kvm")
	Platform string

	// containers tracks active gVisor containers
	containers sync.Map // domain.SandboxID -> *gvisorContainer
}

type gvisorContainer struct {
	ID          domain.SandboxID
	SandboxID   string // String representation for runsc
	BundlePath  string
	Request     *domain.SandboxRequest
	Config      VMConfig
	StartedAt   time.Time
	ExitCode    *int
	Cmd         *exec.Cmd
	ConsoleFile *os.File
	mu          sync.Mutex
}

// NewGVisorRuntime creates a new gVisor runtime instance.
func NewGVisorRuntime(logger *slog.Logger, runscPath, rootDir string) *GVisorRuntime {
	if runscPath == "" {
		runscPath = "/usr/local/bin/runsc"
	}
	if rootDir == "" {
		rootDir = "/var/lib/tartarus/gvisor"
	}

	// Detect platform capability
	platform := "ptrace"
	if _, err := os.Stat("/dev/kvm"); err == nil {
		platform = "kvm"
	}

	return &GVisorRuntime{
		Logger:    logger,
		RunscPath: runscPath,
		RootDir:   rootDir,
		Platform:  platform,
	}
}

// createOCISpec creates an OCI runtime spec for the sandbox
func (g *GVisorRuntime) createOCISpec(req *domain.SandboxRequest, cfg VMConfig) *specs.Spec {
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

// Launch implements SandboxRuntime interface.
func (g *GVisorRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	g.Logger.Info("Launching gVisor sandbox", "id", req.ID)

	sandboxID := string(req.ID)
	bundlePath := filepath.Join(g.RootDir, sandboxID)

	// Create bundle directory
	if err := os.MkdirAll(bundlePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bundle dir: %w", err)
	}

	// Create rootfs directory
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
		"--platform=" + g.Platform,
		"--rootless=false",
		"--network=sandbox",
		"run",
		"--bundle", bundlePath,
		sandboxID,
	}

	cmd := exec.CommandContext(ctx, g.RunscPath, args...)
	cmd.Stdout = consoleFile
	cmd.Stderr = consoleFile
	cmd.Dir = bundlePath

	// Start the sandbox
	if err := cmd.Start(); err != nil {
		consoleFile.Close()
		os.RemoveAll(bundlePath)
		return nil, fmt.Errorf("failed to start runsc: %w", err)
	}

	container := &gvisorContainer{
		ID:          req.ID,
		SandboxID:   sandboxID,
		BundlePath:  bundlePath,
		Request:     req,
		Config:      cfg,
		StartedAt:   time.Now(),
		Cmd:         cmd,
		ConsoleFile: consoleFile,
	}
	g.containers.Store(req.ID, container)

	// Start background goroutine to capture exit status
	go func() {
		err := cmd.Wait()
		container.mu.Lock()
		defer container.mu.Unlock()

		// Close console file once process finishes
		if container.ConsoleFile != nil {
			container.ConsoleFile.Close()
		}

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				container.ExitCode = &code
			} else {
				code := -1
				container.ExitCode = &code
			}
		} else {
			code := 0
			container.ExitCode = &code
		}
	}()

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
	var finishedAt time.Time

	if container.ExitCode != nil {
		if *container.ExitCode == 0 {
			status = domain.RunStatusSucceeded
		} else {
			status = domain.RunStatusFailed
		}
		finishedAt = time.Now() // Approximate, ideally we capture this in the goroutine
	}

	// Try to get memory usage if possible (not implemented here)
	var memUsage domain.Megabytes

	return &domain.SandboxRun{
		ID:          container.ID,
		RequestID:   container.Request.ID,
		NodeID:      container.Request.NodeID,
		Template:    container.Request.Template,
		Status:      status,
		ExitCode:    container.ExitCode,
		StartedAt:   container.StartedAt,
		FinishedAt:  finishedAt,
		UpdatedAt:   time.Now(),
		Metadata:    container.Request.Metadata,
		MemoryUsage: memUsage,
	}, nil
}

// List implements SandboxRuntime interface.
func (g *GVisorRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var runs []domain.SandboxRun

	g.containers.Range(func(key, value interface{}) bool {
		id := key.(domain.SandboxID)
		run, err := g.Inspect(ctx, id)
		if err == nil && run != nil {
			runs = append(runs, *run)
		}
		return true
	})

	return runs, nil
}

// Kill implements SandboxRuntime interface.
func (g *GVisorRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	val, ok := g.containers.Load(id)
	if !ok {
		return nil // Already gone
	}

	container := val.(*gvisorContainer)
	g.Logger.Info("Killing gVisor sandbox", "id", id)

	// Kill using runsc
	killCmd := exec.CommandContext(ctx, g.RunscPath, "kill", container.SandboxID, "KILL")
	if err := killCmd.Run(); err != nil {
		// Try to kill the process directly if runsc fails
		if container.Cmd != nil && container.Cmd.Process != nil {
			_ = container.Cmd.Process.Kill()
		}
	}

	// Delete the sandbox container
	deleteCmd := exec.CommandContext(ctx, g.RunscPath, "delete", "--force", container.SandboxID)
	_ = deleteCmd.Run()

	// Cleanup bundle
	os.RemoveAll(container.BundlePath)

	g.containers.Delete(id)
	return nil
}

// Pause implements SandboxRuntime interface.
func (g *GVisorRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	container := val.(*gvisorContainer)

	cmd := exec.CommandContext(ctx, g.RunscPath, "pause", container.SandboxID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pause sandbox: %w", err)
	}
	return nil
}

// Resume implements SandboxRuntime interface.
func (g *GVisorRuntime) Resume(ctx context.Context, id domain.SandboxID) error {
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	container := val.(*gvisorContainer)

	cmd := exec.CommandContext(ctx, g.RunscPath, "resume", container.SandboxID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to resume sandbox: %w", err)
	}
	return nil
}

// CreateSnapshot implements SandboxRuntime interface.
func (g *GVisorRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	container := val.(*gvisorContainer)

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(memPath), 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// gVisor checkpoint command
	// Note: gVisor checkpoint output is a directory or set of files.
	// We'll treat memPath's directory as the image-path.
	cmd := exec.CommandContext(ctx, g.RunscPath,
		"checkpoint",
		"--image-path", filepath.Dir(memPath),
		container.SandboxID)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkpoint sandbox: %w", err)
	}

	return nil
}

// Shutdown implements SandboxRuntime interface.
func (g *GVisorRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	container := val.(*gvisorContainer)

	// Send SIGTERM
	killCmd := exec.CommandContext(ctx, g.RunscPath, "kill", container.SandboxID, "TERM")
	if err := killCmd.Run(); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for graceful shutdown with timeout

	// Polling for exit
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return g.Kill(ctx, id)
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
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	container := val.(*gvisorContainer)

	// Provide logs from the bundle's console.log
	consolePath := filepath.Join(container.BundlePath, "console.log")
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
				container.mu.Lock()
				done := container.ExitCode != nil
				container.mu.Unlock()
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

// Allocation implements SandboxRuntime interface.
func (g *GVisorRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	var cpu domain.MilliCPU
	var mem domain.Megabytes

	g.containers.Range(func(_, value interface{}) bool {
		container := value.(*gvisorContainer)
		container.mu.Lock()
		if container.ExitCode == nil {
			cpu += container.Request.Resources.CPU
			mem += container.Request.Resources.Mem
		}
		container.mu.Unlock()
		return true
	})

	return domain.ResourceCapacity{
		CPU: cpu,
		Mem: mem,
		GPU: 0,
	}, nil
}

// Wait implements SandboxRuntime interface.
func (g *GVisorRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
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
func (g *GVisorRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	container := val.(*gvisorContainer)

	// Build exec command
	args := append([]string{"exec", container.SandboxID, "--"}, cmd...)
	execCmd := exec.CommandContext(ctx, g.RunscPath, args...)
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to exec: %w", err)
	}

	return nil
}

// ExecInteractive implements SandboxRuntime interface.
func (g *GVisorRuntime) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	val, ok := g.containers.Load(id)
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	container := val.(*gvisorContainer)

	// Build exec command with basic IO forwarding
	// Note: Fully interactive TTY would require --console-socket or specialized handling via --tty which runsc supports
	// Here we keep it simple with standard streams.
	args := append([]string{"exec", container.SandboxID, "--"}, cmd...)
	execCmd := exec.CommandContext(ctx, g.RunscPath, args...)
	execCmd.Stdin = stdin
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to exec interactive: %w", err)
	}
	return nil
}
