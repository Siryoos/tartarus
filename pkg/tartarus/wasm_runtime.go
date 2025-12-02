package tartarus

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// WasmRuntime implements SandboxRuntime using WebAssembly (wazero).
// This provides lightweight, fast-starting sandboxes for WASM workloads.
type WasmRuntime struct {
	Logger *slog.Logger

	// WorkDir is the base directory for WASM module storage and state
	WorkDir string

	// instances tracks active WASM executions
	instances sync.Map // domain.SandboxID -> *wasmInstance

	// runtime is the wazero runtime instance
	runtime wazero.Runtime
}

type wasmInstance struct {
	ID         domain.SandboxID
	Request    *domain.SandboxRequest
	Config     VMConfig
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   *int
	LogPath    string
	ModulePath string
	Cancel     context.CancelFunc
	mu         sync.Mutex
}

// NewWasmRuntime creates a new WASM runtime instance.
func NewWasmRuntime(logger *slog.Logger, workDir string) *WasmRuntime {
	ctx := context.Background()

	// Create wazero runtime with compilation cache
	rt := wazero.NewRuntime(ctx)

	return &WasmRuntime{
		Logger:  logger,
		WorkDir: workDir,
		runtime: rt,
	}
}

// Launch starts a new WASM module execution.
func (w *WasmRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	w.Logger.Info("Launching WASM sandbox", "id", req.ID, "template", req.Template)

	// Create instance directory
	instanceDir := filepath.Join(w.WorkDir, "wasm", string(req.ID))
	if err := os.MkdirAll(instanceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create instance directory: %w", err)
	}

	// Locate WASM module
	modulePath := cfg.Snapshot.Path // Path to .wasm file
	if modulePath == "" {
		return nil, fmt.Errorf("no WASM module path specified in snapshot")
	}
	if _, err := os.Stat(modulePath); err != nil {
		return nil, fmt.Errorf("WASM module not found at %s: %w", modulePath, err)
	}

	// Create log file
	logPath := filepath.Join(instanceDir, "console.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	instance := &wasmInstance{
		ID:         req.ID,
		Request:    req,
		Config:     cfg,
		StartedAt:  time.Now(),
		LogPath:    logPath,
		ModulePath: modulePath,
	}

	// Store instance
	w.instances.Store(req.ID, instance)

	// Launch WASM module in goroutine
	instanceCtx, cancel := context.WithCancel(context.Background())
	instance.Cancel = cancel

	go func() {
		exitCode := w.runWasmModule(instanceCtx, instance)
		instance.mu.Lock()
		instance.ExitCode = &exitCode
		instance.FinishedAt = time.Now()
		instance.mu.Unlock()

		w.Logger.Info("WASM sandbox completed", "id", req.ID, "exit_code", exitCode)
	}()

	// Small delay to check for immediate failures
	time.Sleep(50 * time.Millisecond)

	instance.mu.Lock()
	exitCode := instance.ExitCode
	instance.mu.Unlock()

	if exitCode != nil && *exitCode != 0 {
		return nil, fmt.Errorf("WASM module failed immediately with exit code %d", *exitCode)
	}

	return &domain.SandboxRun{
		ID:        req.ID,
		RequestID: req.ID,
		NodeID:    req.NodeID,
		Template:  req.Template,
		Status:    domain.RunStatusRunning,
		StartedAt: instance.StartedAt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  req.Metadata,
	}, nil
}

// runWasmModule executes the WASM module
func (w *WasmRuntime) runWasmModule(ctx context.Context, inst *wasmInstance) int {
	// Read WASM module
	wasmBytes, err := os.ReadFile(inst.ModulePath)
	if err != nil {
		w.Logger.Error("Failed to read WASM module", "error", err, "path", inst.ModulePath)
		return 1
	}

	// Create module config with WASI
	config := wazero.NewModuleConfig().
		WithStdout(w.getLogWriter(inst.LogPath)).
		WithStderr(w.getLogWriter(inst.LogPath)).
		WithArgs(append([]string{inst.ModulePath}, inst.Request.Args...)...).
		WithStartFunctions("_start")

	// Add environment variables
	for k, v := range inst.Request.Env {
		config = config.WithEnv(k, v)
	}

	// Instantiate WASI
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, w.runtime); err != nil {
		w.Logger.Error("Failed to instantiate WASI", "error", err)
		return 1
	}

	// Compile and instantiate module
	mod, err := w.runtime.InstantiateWithConfig(ctx, wasmBytes, config)
	if err != nil {
		w.Logger.Error("Failed to instantiate WASM module", "error", err)
		return 1
	}
	defer mod.Close(ctx)

	// Module execution happens during instantiation with _start
	// If we reached here, execution completed successfully
	return 0
}

func (w *WasmRuntime) getLogWriter(logPath string) io.Writer {
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		w.Logger.Error("Failed to open log file", "error", err)
		return os.Stdout
	}
	return f
}

// Inspect returns the current state of a WASM sandbox.
func (w *WasmRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	val, ok := w.instances.Load(id)
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}

	inst := val.(*wasmInstance)
	inst.mu.Lock()
	defer inst.mu.Unlock()

	status := domain.RunStatusRunning
	if inst.ExitCode != nil {
		if *inst.ExitCode == 0 {
			status = domain.RunStatusSucceeded
		} else {
			status = domain.RunStatusFailed
		}
	}

	return &domain.SandboxRun{
		ID:         inst.ID,
		RequestID:  inst.Request.ID,
		NodeID:     inst.Request.NodeID,
		Template:   inst.Request.Template,
		Status:     status,
		ExitCode:   inst.ExitCode,
		StartedAt:  inst.StartedAt,
		FinishedAt: inst.FinishedAt,
		CreatedAt:  inst.StartedAt,
		UpdatedAt:  time.Now(),
		Metadata:   inst.Request.Metadata,
	}, nil
}

// List returns all active WASM sandboxes.
func (w *WasmRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var runs []domain.SandboxRun

	w.instances.Range(func(key, value interface{}) bool {
		inst := value.(*wasmInstance)
		inst.mu.Lock()

		status := domain.RunStatusRunning
		if inst.ExitCode != nil {
			if *inst.ExitCode == 0 {
				status = domain.RunStatusSucceeded
			} else {
				status = domain.RunStatusFailed
			}
		}

		runs = append(runs, domain.SandboxRun{
			ID:         inst.ID,
			RequestID:  inst.Request.ID,
			NodeID:     inst.Request.NodeID,
			Template:   inst.Request.Template,
			Status:     status,
			ExitCode:   inst.ExitCode,
			StartedAt:  inst.StartedAt,
			FinishedAt: inst.FinishedAt,
			CreatedAt:  inst.StartedAt,
			UpdatedAt:  time.Now(),
			Metadata:   inst.Request.Metadata,
		})

		inst.mu.Unlock()
		return true
	})

	return runs, nil
}

// Kill terminates a WASM sandbox.
func (w *WasmRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	val, ok := w.instances.Load(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}

	inst := val.(*wasmInstance)
	if inst.Cancel != nil {
		inst.Cancel()
	}

	// Remove from active instances
	w.instances.Delete(id)

	w.Logger.Info("WASM sandbox killed", "id", id)
	return nil
}

// Pause is not supported for WASM runtime.
func (w *WasmRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	return fmt.Errorf("pause not supported for WASM runtime")
}

// Resume is not supported for WASM runtime.
func (w *WasmRuntime) Resume(ctx context.Context, id domain.SandboxID) error {
	return fmt.Errorf("resume not supported for WASM runtime")
}

// CreateSnapshot is not supported for WASM runtime.
func (w *WasmRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	return fmt.Errorf("snapshots not supported for WASM runtime")
}

// Shutdown gracefully stops a WASM sandbox (same as Kill for WASM).
func (w *WasmRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	return w.Kill(ctx, id)
}

// GetConfig returns the configuration used to launch the sandbox.
func (w *WasmRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (VMConfig, *domain.SandboxRequest, error) {
	val, ok := w.instances.Load(id)
	if !ok {
		return VMConfig{}, nil, fmt.Errorf("sandbox not found: %s", id)
	}

	inst := val.(*wasmInstance)
	return inst.Config, inst.Request, nil
}

// StreamLogs streams logs from the WASM sandbox.
func (w *WasmRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, writer io.Writer, follow bool) error {
	val, ok := w.instances.Load(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}

	inst := val.(*wasmInstance)

	// Open log file
	f, err := os.Open(inst.LogPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	if !follow {
		// Just copy existing logs
		_, err = io.Copy(writer, f)
		return err
	}

	// Follow mode: tail the log file
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			n, err := f.Read(buf)
			if n > 0 {
				if _, werr := writer.Write(buf[:n]); werr != nil {
					return werr
				}
			}
			if err == io.EOF {
				// Check if instance is still running
				inst.mu.Lock()
				finished := inst.ExitCode != nil
				inst.mu.Unlock()

				if finished {
					return nil
				}

				// Wait a bit before retrying
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if err != nil {
				return err
			}
		}
	}
}

// Allocation returns resource allocation for WASM runtime.
func (w *WasmRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	// WASM has minimal resource overhead
	// Rough estimate: each instance uses ~10MB memory
	count := 0
	w.instances.Range(func(_, _ interface{}) bool {
		count++
		return true
	})

	return domain.ResourceCapacity{
		CPU: domain.MilliCPU(count * 100), // 0.1 CPU per instance
		Mem: domain.Megabytes(count * 10), // 10MB per instance
		GPU: 0,
	}, nil
}

// Wait blocks until the sandbox completes.
func (w *WasmRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	val, ok := w.instances.Load(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}

	inst := val.(*wasmInstance)

	// Poll for completion
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			inst.mu.Lock()
			finished := inst.ExitCode != nil
			inst.mu.Unlock()

			if finished {
				return nil
			}
		}
	}
}

// Exec is not supported for WASM runtime.
func (w *WasmRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string) error {
	return fmt.Errorf("exec not supported for WASM runtime")
}
