//go:build linux
// +build linux

package tartarus

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// FirecrackerRuntime implements SandboxRuntime using the Firecracker SDK.
type FirecrackerRuntime struct {
	Logger *slog.Logger

	// Root paths
	SocketDir   string
	KernelImage string
	RootFSBase  string

	// State tracking: SandboxID -> *vmState
	vms sync.Map
}

type vmState struct {
	Machine     *firecracker.Machine
	Cmd         *exec.Cmd
	SocketPath  string
	LogPath     string
	ConsolePath string
	StartedAt   time.Time
	Request     *domain.SandboxRequest
	Config      VMConfig
	ExitCode    *int
	mu          sync.Mutex
}

func (r *FirecrackerRuntime) getState(id domain.SandboxID) (*vmState, error) {
	val, ok := r.vms.Load(id)
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	return val.(*vmState), nil
}

// NewFirecrackerRuntime creates a new runtime instance.
func NewFirecrackerRuntime(logger *slog.Logger, socketDir, kernelImage, rootFSBase string) *FirecrackerRuntime {
	return &FirecrackerRuntime{
		Logger:      logger,
		SocketDir:   socketDir,
		KernelImage: kernelImage,
		RootFSBase:  rootFSBase,
	}
}

// Launch starts a new Firecracker microVM.
func (r *FirecrackerRuntime) Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error) {
	r.Logger.Info("Launching Firecracker VM", "id", req.ID)

	// Ensure socket directory exists
	if err := os.MkdirAll(r.SocketDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create socket dir: %w", err)
	}

	socketPath := filepath.Join(r.SocketDir, fmt.Sprintf("fc-%s.sock", req.ID))
	logPath := filepath.Join(r.SocketDir, fmt.Sprintf("fc-%s.log", req.ID))
	consolePath := filepath.Join(r.SocketDir, fmt.Sprintf("fc-%s.console", req.ID))

	// Determine RootFS path
	// If cfg.OverlayFS is set, use it. Otherwise use RootFSBase (or cfg.Snapshot.Path if we had it)
	// For this PR, we'll prefer OverlayFS if present, else RootFSBase.
	rootFSPath := r.RootFSBase
	if cfg.OverlayFS != "" {
		rootFSPath = cfg.OverlayFS
	}

	// Helper to convert MB to int64
	memSz := int64(req.Resources.Mem)
	if memSz == 0 {
		memSz = 128 // Default to 128MB
	}
	cpuCount := int64(req.Resources.CPU) / 1000
	if cpuCount < 1 {
		cpuCount = 1
	}

	// Construct Kernel Args
	// We want: console=ttyS0 reboot=k panic=1 pci=off init=/bin/sh -- -c "export VAR=VAL; exec cmd args..."
	// Note: We use /bin/sh to setup env and exec the actual command.
	kernelArgs := "console=ttyS0 reboot=k panic=1 pci=off"

	if len(req.Command) > 0 {
		// Build the shell script
		var scriptBuilder strings.Builder

		// 0. Configure Network (if provided)
		if cfg.IP.IsValid() {
			// ip addr add <IP>/<MASK> dev eth0
			// ip link set eth0 up
			// ip route add default via <GATEWAY>

			// Calculate mask from CIDR
			// cfg.CIDR is the network prefix, e.g. 10.200.0.0/16
			// We want the mask bits from it.
			bits := cfg.CIDR.Bits()

			scriptBuilder.WriteString(fmt.Sprintf("ip addr add %s/%d dev eth0; ", cfg.IP, bits))
			scriptBuilder.WriteString("ip link set eth0 up; ")
			if cfg.Gateway.IsValid() {
				scriptBuilder.WriteString(fmt.Sprintf("ip route add default via %s; ", cfg.Gateway))
			}
		}

		// 1. Export Environment Variables
		for k, v := range req.Env {
			// Simple escaping for single quotes
			val := strings.ReplaceAll(v, "'", "'\\''")
			scriptBuilder.WriteString(fmt.Sprintf("export %s='%s'; ", k, val))
		}

		// 2. Build the command
		// We assume req.Command[0] is the binary, and req.Args are arguments.
		// If req.Args is empty, we just use req.Command.
		// Actually domain.SandboxRequest has Command []string and Args []string.
		// Usually Command is ["/bin/prog"] and Args is ["-flag", "val"].
		// We'll combine them.
		fullCmd := append(req.Command, req.Args...)

		// Exec the command so it takes over PID 1 (or whatever sh was)
		scriptBuilder.WriteString("exec")
		for _, part := range fullCmd {
			// Escape arguments
			arg := strings.ReplaceAll(part, "'", "'\\''")
			scriptBuilder.WriteString(fmt.Sprintf(" '%s'", arg))
		}

		// Append init=/bin/sh and the script
		// We pass the script as an argument to sh -c
		// We use double quotes for the kernel command line to group the script as one argument.
		// We must escape double quotes in the script itself.
		script := scriptBuilder.String()
		scriptEscaped := strings.ReplaceAll(script, "\"", "\\\"")

		kernelArgs = fmt.Sprintf("%s init=/bin/sh -- -c \"%s\"", kernelArgs, scriptEscaped)
	}

	fcCfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: r.KernelImage,
		KernelArgs:      kernelArgs,
		LogPath:         logPath,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(cpuCount),
			MemSizeMib: firecracker.Int64(memSz),
			Smt:        firecracker.Bool(false),
		},
		Drives: []models.Drive{
			{
				DriveID:      firecracker.String("rootfs"),
				PathOnHost:   firecracker.String(rootFSPath),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
			},
		},
	}

	// Add Network Interface if TapDevice is provided
	if cfg.TapDevice != "" {
		fcCfg.NetworkInterfaces = []firecracker.NetworkInterface{
			{
				StaticConfiguration: &firecracker.StaticNetworkConfiguration{
					HostDevName: cfg.TapDevice,
					// MacAddress: ... (optional, can be generated or assigned)
				},
			},
		}
	}

	// Create console log file
	consoleFile, err := os.Create(consolePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create console log: %w", err)
	}
	// We don't close consoleFile here; we pass it to the cmd.
	// The cmd will hold it open. We can close it in Kill or when cmd finishes?
	// Actually exec.Cmd doesn't close Stdout/Stderr if they are *os.File.
	// We should close it after the process starts? No, then the process can't write to it?
	// Wait, if we pass *os.File to Cmd.Stdout, the child inherits the fd.
	// We can close our handle after Start().
	// But we are using firecracker.NewMachine which calls Start internally?
	// No, NewMachine just creates the struct. machine.Start() starts it.
	// But we build the cmd first.

	cmd := firecracker.VMCommandBuilder{}.
		WithSocketPath(socketPath).
		Build(ctx)

	cmd.Stdout = consoleFile
	cmd.Stderr = consoleFile

	// Check if we are restoring from a snapshot
	if cfg.Snapshot.Path != "" {
		r.Logger.Info("Restoring from snapshot", "id", req.ID, "snapshot", cfg.Snapshot.Path)
		fcCfg.Snapshot = firecracker.SnapshotConfig{
			MemFilePath:         cfg.Snapshot.Path + ".mem",
			SnapshotPath:        cfg.Snapshot.Path + ".disk",
			EnableDiffSnapshots: false,
			ResumeVM:            true,
		}
		// Clear KernelImagePath as we are restoring
		fcCfg.KernelImagePath = ""
		// KernelArgs might be ignored during restore?
		// Usually yes, the VM state includes memory and cpu state.
		// So we can't change the command when restoring from a snapshot unless the snapshot was paused at bootloader?
		// Firecracker snapshots are full system state.
		// So we can't inject a new command into a restored snapshot.
		// The request should probably match the snapshot's original intent or we just resume it.
		// For now, we assume if snapshot is present, we just resume it.
	}

	machine, err := firecracker.NewMachine(ctx, fcCfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		consoleFile.Close()
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}

	if err := machine.Start(ctx); err != nil {
		consoleFile.Close()
		return nil, fmt.Errorf("failed to start machine: %w", err)
	}

	// Close our handle to the console file, the child process has its own.
	consoleFile.Close()

	// Store state
	state := &vmState{
		Machine:     machine,
		Cmd:         cmd,
		SocketPath:  socketPath,
		LogPath:     logPath,
		ConsolePath: consolePath,
		StartedAt:   time.Now(),
		Request:     req,
		Config:      cfg,
	}
	r.vms.Store(req.ID, state)

	return &domain.SandboxRun{
		ID:        req.ID,
		RequestID: req.ID,
		Status:    domain.RunStatusRunning,
		StartedAt: state.StartedAt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (r *FirecrackerRuntime) Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	val, ok := r.vms.Load(id)
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	state := val.(*vmState)

	state.mu.Lock()
	defer state.mu.Unlock()

	status := domain.RunStatusRunning
	if state.ExitCode != nil {
		if *state.ExitCode == 0 {
			status = domain.RunStatusSucceeded
		} else {
			status = domain.RunStatusFailed
		}
	}

	// Get memory usage
	var memUsage domain.Megabytes
	if state.Cmd != nil && state.Cmd.Process != nil && status == domain.RunStatusRunning {
		// Read /proc/<pid>/statm
		// Format: size resident shared text lib data dt
		// resident is in pages
		pid := state.Cmd.Process.Pid
		statmPath := fmt.Sprintf("/proc/%d/statm", pid)
		if content, err := os.ReadFile(statmPath); err == nil {
			fields := strings.Fields(string(content))
			if len(fields) >= 2 {
				var rssPages int64
				fmt.Sscanf(fields[1], "%d", &rssPages)
				// Assume 4KB pages
				memUsage = domain.Megabytes(rssPages * 4 / 1024)
			}
		}
	}

	return &domain.SandboxRun{
		ID:          state.Request.ID,
		RequestID:   state.Request.ID,
		Status:      status,
		ExitCode:    state.ExitCode,
		StartedAt:   state.StartedAt,
		UpdatedAt:   time.Now(),
		MemoryUsage: memUsage,
	}, nil
}

func (r *FirecrackerRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var list []domain.SandboxRun
	r.vms.Range(func(key, value any) bool {
		state := value.(*vmState)
		state.mu.Lock()
		status := domain.RunStatusRunning
		if state.ExitCode != nil {
			if *state.ExitCode == 0 {
				status = domain.RunStatusSucceeded
			} else {
				status = domain.RunStatusFailed
			}
		}
		state.mu.Unlock()

		list = append(list, domain.SandboxRun{
			ID:        state.Request.ID,
			RequestID: state.Request.ID,
			Status:    status,
			ExitCode:  state.ExitCode,
			StartedAt: state.StartedAt,
			UpdatedAt: time.Now(),
		})
		return true
	})
	return list, nil
}

func (r *FirecrackerRuntime) Kill(ctx context.Context, id domain.SandboxID) error {
	val, ok := r.vms.Load(id)
	if !ok {
		return nil // Already gone
	}
	state := val.(*vmState)

	r.Logger.Info("Killing sandbox", "id", id)

	// Stop VMM
	if err := state.Machine.StopVMM(); err != nil {
		r.Logger.Warn("StopVMM failed", "error", err)
	}

	// Clean up
	r.vms.Delete(id)
	os.Remove(state.SocketPath)
	// We keep the log/console files for debugging/streaming?
	// If we delete them, StreamLogs might fail if called after Kill.
	// Usually we might want to keep them for a bit or let a reaper clean them up.
	// For now, we'll leave them.

	return nil
}

func (r *FirecrackerRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer) error {
	val, ok := r.vms.Load(id)
	if !ok {
		return fmt.Errorf("sandbox not found")
	}
	state := val.(*vmState)

	// Tail the console log file
	file, err := os.Open(state.ConsolePath)
	if err != nil {
		return err
	}
	defer file.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	buf := make([]byte, 1024)
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
				// Check if process is still running
				state.mu.Lock()
				done := state.ExitCode != nil
				state.mu.Unlock()
				if done {
					// If done and EOF, we are finished
					return nil
				}
				// Else wait for more logs
				continue
			}
			if err != nil {
				return err
			}
		}
	}
}

func (r *FirecrackerRuntime) Allocation(ctx context.Context) (domain.ResourceCapacity, error) {
	var cpu domain.MilliCPU
	var mem domain.Megabytes

	r.vms.Range(func(key, value any) bool {
		state := value.(*vmState)
		// Only count running VMs?
		state.mu.Lock()
		if state.ExitCode == nil {
			cpu += domain.MilliCPU(state.Config.CPUs * 1000)
			mem += domain.Megabytes(state.Config.MemoryMB)
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

func (r *FirecrackerRuntime) Wait(ctx context.Context, id domain.SandboxID) error {
	val, ok := r.vms.Load(id)
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	state := val.(*vmState)

	// Wait for the VM to exit.
	err := state.Machine.Wait(ctx)

	// Capture exit code
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Cmd.ProcessState != nil {
		code := state.Cmd.ProcessState.ExitCode()
		state.ExitCode = &code
	} else {
		// Should not happen if Wait returned?
		// Unless Wait returned error before process started?
		// Or if Wait doesn't populate ProcessState?
		// exec.Cmd.Wait() populates ProcessState.
		// firecracker-go-sdk Machine.Wait() calls cmd.Wait().
		// So it should be there.
		// If err != nil, it might still be there.
		if state.Cmd.ProcessState != nil {
			code := state.Cmd.ProcessState.ExitCode()
			state.ExitCode = &code
		} else {
			// Fallback
			c := -1
			state.ExitCode = &c
		}
	}

	return err
}

func (r *FirecrackerRuntime) Pause(ctx context.Context, id domain.SandboxID) error {
	state, err := r.getState(id)
	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Machine == nil {
		return fmt.Errorf("machine not initialized for %s", id)
	}

	return state.Machine.PauseVM(ctx)
}

func (r *FirecrackerRuntime) Resume(ctx context.Context, id domain.SandboxID) error {
	state, err := r.getState(id)
	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Machine == nil {
		return fmt.Errorf("machine not initialized for %s", id)
	}

	return state.Machine.ResumeVM(ctx)
}

func (r *FirecrackerRuntime) CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error {
	if err := os.MkdirAll(filepath.Dir(memPath), 0755); err != nil {
		return fmt.Errorf("failed to create snapshot dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
		return fmt.Errorf("failed to create snapshot dir: %w", err)
	}

	state, err := r.getState(id)
	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Machine == nil {
		return fmt.Errorf("machine not initialized for %s", id)
	}

	return state.Machine.CreateSnapshot(ctx, memPath, diskPath)
}

func (r *FirecrackerRuntime) Shutdown(ctx context.Context, id domain.SandboxID) error {
	state, err := r.getState(id)
	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Machine == nil {
		return fmt.Errorf("machine not initialized for %s", id)
	}

	return state.Machine.Shutdown(ctx)
}

func (r *FirecrackerRuntime) GetConfig(ctx context.Context, id domain.SandboxID) (VMConfig, *domain.SandboxRequest, error) {
	state, err := r.getState(id)
	if err != nil {
		return VMConfig{}, nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Request == nil {
		return VMConfig{}, nil, fmt.Errorf("sandbox %s missing request metadata", id)
	}

	cfgCopy := state.Config
	reqCopy := *state.Request

	return cfgCopy, &reqCopy, nil
}
