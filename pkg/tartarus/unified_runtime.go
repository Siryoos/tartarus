package tartarus

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// IsolationType defines the type of isolation/runtime to use.
type IsolationType string

const (
	IsolationMicroVM IsolationType = "microvm"
	IsolationWASM    IsolationType = "wasm"
	IsolationGVisor  IsolationType = "gvisor"
	IsolationAuto    IsolationType = "auto"
)

// UnifiedRuntime provides a unified interface across multiple runtime backends
// with automatic runtime selection based on workload characteristics.
type UnifiedRuntime struct {
	Logger *slog.Logger

	// Runtime backends
	microVM SandboxRuntime
	wasm    SandboxRuntime
	gvisor  SandboxRuntime

	// Default runtime when auto-selection is disabled
	defaultRuntime IsolationType

	// Auto-selection enabled
	autoSelect bool

	// Selector implements the auto-selection logic
	selector *RuntimeSelector

	// Metrics
	metrics *hermes.PrometheusMetrics
}

// UnifiedRuntimeConfig configures the unified runtime.
type UnifiedRuntimeConfig struct {
	// Runtime instances
	MicroVMRuntime SandboxRuntime
	WasmRuntime    SandboxRuntime
	GVisorRuntime  SandboxRuntime

	// Default runtime to use
	DefaultRuntime IsolationType

	// Enable automatic runtime selection
	AutoSelect bool

	Logger *slog.Logger

	Metrics *hermes.PrometheusMetrics
}

// NewUnifiedRuntime creates a new unified runtime instance.
func NewUnifiedRuntime(cfg UnifiedRuntimeConfig) *UnifiedRuntime {
	return &UnifiedRuntime{
		Logger:         cfg.Logger,
		microVM:        cfg.MicroVMRuntime,
		wasm:           cfg.WasmRuntime,
		gvisor:         cfg.GVisorRuntime,
		defaultRuntime: cfg.DefaultRuntime,
		autoSelect:     cfg.AutoSelect,
		selector:       NewRuntimeSelector(cfg.Logger),
		metrics:        cfg.Metrics,
	}
}

// selectRuntime chooses the appropriate runtime for a sandbox request.
func (u *UnifiedRuntime) selectRuntime(req *domain.SandboxRequest) (SandboxRuntime, IsolationType, error) {
	// Check if request explicitly specifies isolation type
	if req.Metadata != nil {
		if isolationType, ok := req.Metadata["isolation_type"]; ok {
			return u.getRuntimeByType(IsolationType(isolationType))
		}
	}

	// If auto-selection is disabled, use default runtime
	if !u.autoSelect {
		return u.getRuntimeByType(u.defaultRuntime)
	}

	// Use auto-selection logic
	// Use auto-selection logic
	selectedType := u.selector.SelectRuntime(req)

	if u.metrics != nil {
		u.metrics.IncCounter("tartarus_runtime_selection_total", 1,
			hermes.Label{Key: "source", Value: "auto"},
			hermes.Label{Key: "selected_runtime", Value: string(selectedType)},
		)
	}

	return u.getRuntimeByType(selectedType)
}

// getRuntimeByType returns the runtime instance for the given type.
func (u *UnifiedRuntime) getRuntimeByType(isoType IsolationType) (SandboxRuntime, IsolationType, error) {
	switch isoType {
	case IsolationMicroVM:
		if u.microVM == nil {
			return nil, "", fmt.Errorf("microVM runtime not available")
		}
		return u.microVM, IsolationMicroVM, nil

	case IsolationWASM:
		if u.wasm == nil {
			return nil, "", fmt.Errorf("WASM runtime not available")
		}
		return u.wasm, IsolationWASM, nil

	case IsolationGVisor:
		if u.gvisor == nil {
			return nil, "", fmt.Errorf("gVisor runtime not available")
		}
		return u.gvisor, IsolationGVisor, nil

	case IsolationAuto:
		// Shouldn't reach here, but default to microVM
		if u.microVM != nil {
			return u.microVM, IsolationMicroVM, nil
		}
		return nil, "", fmt.Errorf("no runtime available")

	default:
		return nil, "", fmt.Errorf("unknown isolation type: %s", isoType)
	}
}

// Launch implements SandboxRuntime interface with runtime selection.
func (u *UnifiedRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	runtime, isoType, err := u.selectRuntime(req)
	if err != nil {
		return nil, fmt.Errorf("runtime selection failed: %w", err)
	}

	u.Logger.Info("Launching sandbox", "id", req.ID, "runtime", isoType)

	run, err := runtime.Launch(ctx, req, cfg)
	if err != nil {
		return nil, err
	}

	// Record metrics
	if u.metrics != nil {
		u.metrics.IncCounter("tartarus_sandbox_count", 1, hermes.Label{Key: "runtime", Value: string(isoType)})
	}

	// Add runtime type to metadata
	if run.Metadata == nil {
		run.Metadata = make(map[string]string)
	}
	run.Metadata["runtime_type"] = string(isoType)

	return run, nil
}

// Inspect implements SandboxRuntime interface.
// It tries all runtimes to find the sandbox.
func (u *UnifiedRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	// Try each runtime in order of likelihood
	runtimes := []struct {
		runtime SandboxRuntime
		name    string
	}{
		{u.microVM, "microvm"},
		{u.wasm, "wasm"},
		{u.gvisor, "gvisor"},
	}

	for _, rt := range runtimes {
		if rt.runtime == nil {
			continue
		}

		run, err := rt.runtime.Inspect(ctx, id)
		if err == nil {
			// Found it
			if run.Metadata == nil {
				run.Metadata = make(map[string]string)
			}
			run.Metadata["runtime_type"] = rt.name
			return run, nil
		}
	}

	return nil, fmt.Errorf("sandbox not found: %s", id)
}

// List implements SandboxRuntime interface.
// It aggregates sandboxes from all runtimes.
func (u *UnifiedRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var allRuns []domain.SandboxRun

	runtimes := []struct {
		runtime SandboxRuntime
		name    string
	}{
		{u.microVM, "microvm"},
		{u.wasm, "wasm"},
		{u.gvisor, "gvisor"},
	}

	for _, rt := range runtimes {
		if rt.runtime == nil {
			continue
		}

		runs, err := rt.runtime.List(ctx)
		if err != nil {
			u.Logger.Error("Failed to list from runtime", "runtime", rt.name, "error", err)
			continue
		}

		// Add runtime type to metadata
		for i := range runs {
			if runs[i].Metadata == nil {
				runs[i].Metadata = make(map[string]string)
			}
			runs[i].Metadata["runtime_type"] = rt.name
		}

		allRuns = append(allRuns, runs...)
	}

	return allRuns, nil
}

// delegateToRuntime finds the runtime that owns this sandbox and delegates the operation.
func (u *UnifiedRuntime) delegateToRuntime(ctx context.Context, id domain.SandboxID, operation string) (SandboxRuntime, error) {
	// First try to inspect to find which runtime owns it
	run, err := u.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check metadata for runtime type
	if run.Metadata != nil {
		if rtType, ok := run.Metadata["runtime_type"]; ok {
			rt, _, err := u.getRuntimeByType(IsolationType(rtType))
			if err == nil {
				return rt, nil
			}
		}
	}

	// Fallback: try all runtimes
	runtimes := []SandboxRuntime{u.microVM, u.wasm, u.gvisor}
	for _, rt := range runtimes {
		if rt == nil {
			continue
		}

		// Try inspect to see if this runtime owns it
		_, err := rt.Inspect(ctx, id)
		if err == nil {
			return rt, nil
		}
	}

	return nil, fmt.Errorf("sandbox not found in any runtime: %s", id)
}

// Kill implements SandboxRuntime interface.
func (u *UnifiedRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	runtime, err := u.delegateToRuntime(ctx, id, "kill")
	if err != nil {
		return err
	}
	return runtime.Kill(ctx, id)
}

// Pause implements SandboxRuntime interface.
func (u *UnifiedRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	runtime, err := u.delegateToRuntime(ctx, id, "pause")
	if err != nil {
		return err
	}
	return runtime.Pause(ctx, id)
}

// Resume implements SandboxRuntime interface.
func (u *UnifiedRuntime) Resume(ctx context.Context, id domain.SandboxID) error {
	runtime, err := u.delegateToRuntime(ctx, id, "resume")
	if err != nil {
		return err
	}
	return runtime.Resume(ctx, id)
}

// CreateSnapshot implements SandboxRuntime interface.
func (u *UnifiedRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	runtime, err := u.delegateToRuntime(ctx, id, "snapshot")
	if err != nil {
		return err
	}
	return runtime.CreateSnapshot(ctx, id, memPath, diskPath)
}

// Shutdown implements SandboxRuntime interface.
func (u *UnifiedRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	runtime, err := u.delegateToRuntime(ctx, id, "shutdown")
	if err != nil {
		return err
	}
	return runtime.Shutdown(ctx, id)
}

// GetConfig implements SandboxRuntime interface.
func (u *UnifiedRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (VMConfig, *domain.SandboxRequest, error) {
	runtime, err := u.delegateToRuntime(ctx, id, "getconfig")
	if err != nil {
		return VMConfig{}, nil, err
	}
	return runtime.GetConfig(ctx, id)
}

// StreamLogs implements SandboxRuntime interface.
func (u *UnifiedRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	runtime, err := u.delegateToRuntime(ctx, id, "logs")
	if err != nil {
		return err
	}
	return runtime.StreamLogs(ctx, id, w, follow)
}

// Allocation implements SandboxRuntime interface.
// Returns aggregate allocation across all runtimes.
func (u *UnifiedRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	total := domain.ResourceCapacity{}

	runtimes := []SandboxRuntime{u.microVM, u.wasm, u.gvisor}
	for _, rt := range runtimes {
		if rt == nil {
			continue
		}

		alloc, err := rt.Allocation(ctx)
		if err != nil {
			u.Logger.Error("Failed to get allocation from runtime", "error", err)
			continue
		}

		total.CPU += alloc.CPU
		total.Mem += alloc.Mem
		total.GPU += alloc.GPU
	}

	return total, nil
}

// Wait implements SandboxRuntime interface.
func (u *UnifiedRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	runtime, err := u.delegateToRuntime(ctx, id, "wait")
	if err != nil {
		return err
	}
	return runtime.Wait(ctx, id)
}

// Exec implements SandboxRuntime interface.
func (u *UnifiedRuntime) Exec(ctx context.Context, id domain.SandboxID, cmd []string, stdout, stderr io.Writer) error {
	runtime, err := u.delegateToRuntime(ctx, id, "exec")
	if err != nil {
		return err
	}
	return runtime.Exec(ctx, id, cmd, stdout, stderr)
}

// ExecInteractive implements SandboxRuntime interface.
func (u *UnifiedRuntime) ExecInteractive(ctx context.Context, id domain.SandboxID, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	runtime, err := u.delegateToRuntime(ctx, id, "exec_interactive")
	if err != nil {
		return err
	}
	return runtime.ExecInteractive(ctx, id, cmd, stdin, stdout, stderr)
}

// RuntimeSelector implements automatic runtime selection logic.
type RuntimeSelector struct {
	Logger *slog.Logger
}

// NewRuntimeSelector creates a new runtime selector.
func NewRuntimeSelector(logger *slog.Logger) *RuntimeSelector {
	return &RuntimeSelector{Logger: logger}
}

// SelectRuntime chooses the best runtime for a given sandbox request.
func (s *RuntimeSelector) SelectRuntime(req *domain.SandboxRequest) IsolationType {
	// Decision criteria:

	// 1. Check template hints
	if req.Metadata != nil {
		if hint, ok := req.Metadata["preferred_runtime"]; ok {
			switch hint {
			case "wasm":
				return IsolationWASM
			case "microvm":
				return IsolationMicroVM
			case "gvisor":
				return IsolationGVisor
			}
		}
	}

	// 2. Resource-based selection
	// WASM: For lightweight, short-lived tasks
	if s.isLightweight(req) {
		s.Logger.Info("Auto-selecting WASM runtime", "reason", "lightweight workload")
		return IsolationWASM
	}

	// 3. Check for privileged operations or GPU (requires microVM)
	if s.requiresPrivileged(req) {
		s.Logger.Info("Auto-selecting microVM runtime", "reason", "privileged/GPU workload")
		return IsolationMicroVM
	}

	// 4. Default to microVM for general-purpose workloads
	s.Logger.Info("Auto-selecting microVM runtime", "reason", "default")
	return IsolationMicroVM
}

// isLightweight checks if a workload is lightweight enough for WASM.
func (s *RuntimeSelector) isLightweight(req *domain.SandboxRequest) bool {
	// Criteria for WASM suitability:
	// - Low CPU (< 500 milliCPU = 0.5 cores)
	// - Low memory (< 256MB)
	// - Short TTL (< 5 minutes)
	// - No GPU

	if req.Resources.CPU > 500 {
		return false
	}

	if req.Resources.Mem > 256 {
		return false
	}

	if req.Resources.GPU.Count > 0 {
		return false
	}

	// If TTL is very short, likely a function
	if req.Resources.TTL > 0 && req.Resources.TTL < 5*60*1000000000 { // 5 minutes in nanoseconds
		return true
	}

	// Small resource footprint suggests WASM suitability
	if req.Resources.CPU <= 250 && req.Resources.Mem <= 128 {
		return true
	}

	return false
}

// requiresPrivileged checks if a workload requires privileged access.
func (s *RuntimeSelector) requiresPrivileged(req *domain.SandboxRequest) bool {
	// Requires microVM if:
	// - GPU requested
	// - High resource requirements
	// - Specific metadata hints

	if req.Resources.GPU.Count > 0 {
		return true
	}

	// High resource workloads benefit from microVM isolation
	if req.Resources.CPU > 2000 || req.Resources.Mem > 4096 {
		return true
	}

	if req.Metadata != nil {
		if _, needsKernel := req.Metadata["kernel_modules"]; needsKernel {
			return true
		}
		if _, needsDevices := req.Metadata["devices"]; needsDevices {
			return true
		}
	}

	return false
}
