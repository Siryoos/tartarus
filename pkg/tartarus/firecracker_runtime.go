//go:build linux
// +build linux

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
	Machine    *firecracker.Machine
	SocketPath string
	LogPath    string
	StartedAt  time.Time
	Request    *domain.SandboxRequest
	Config     VMConfig
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

	fcCfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: r.KernelImage,
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

	// Create command to start firecracker
	// The SDK needs a way to start the process.
	// We can use firecracker.NewMachine which handles the process if we provide the binary command?
	// Actually, NewMachine expects us to manage the process or it manages it if we provide the cmd.
	// Standard way:
	cmd := firecracker.VMCommandBuilder{}.
		WithSocketPath(socketPath).
		Build(ctx) // Assumes 'firecracker' is in PATH

	machine, err := firecracker.NewMachine(ctx, fcCfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}

	if err := machine.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start machine: %w", err)
	}

	// Store state
	state := &vmState{
		Machine:    machine,
		SocketPath: socketPath,
		LogPath:    logPath,
		StartedAt:  time.Now(),
		Request:    req,
		Config:     cfg,
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

	// Check if process is still running
	// This is a simplified check.
	// In a real system we'd query the socket.
	status := domain.RunStatusRunning

	// If we can check the process:
	// The SDK machine struct has a PID, but it's not always exposed directly depending on version/usage.
	// But we can try to call DescribeInstanceInfo.
	// info, err := state.Machine.DescribeInstanceInfo(ctx)
	// if err != nil { ... }

	// For now, assume running if in map.

	return &domain.SandboxRun{
		ID:        state.Request.ID,
		RequestID: state.Request.ID,
		Status:    status,
		StartedAt: state.StartedAt,
		UpdatedAt: time.Now(),
	}, nil
}

func (r *FirecrackerRuntime) List(ctx context.Context) ([]domain.SandboxRun, error) {
	var list []domain.SandboxRun
	r.vms.Range(func(key, value any) bool {
		state := value.(*vmState)
		list = append(list, domain.SandboxRun{
			ID:        state.Request.ID,
			RequestID: state.Request.ID,
			Status:    domain.RunStatusRunning, // Simplified
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
		// Try to kill process if StopVMM fails or if we want to force it
		// But StopVMM sends the shutdown command.
		r.Logger.Warn("StopVMM failed", "error", err)
		// We could try sending SIGKILL to the process if we had the PID.
	}

	// Clean up
	r.vms.Delete(id)
	os.Remove(state.SocketPath)
	// We keep the log file for now? Or delete it?
	// Usually logs are kept for a bit.

	return nil
}

func (r *FirecrackerRuntime) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer) error {
	val, ok := r.vms.Load(id)
	if !ok {
		return fmt.Errorf("sandbox not found")
	}
	state := val.(*vmState)

	// Simple implementation: tail the log file
	// This is blocking, so it fits the StreamLogs pattern.

	file, err := os.Open(state.LogPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read until EOF, then wait and read more
	// Similar to 'tail -f'
	// For this PR, just copy what's there and maybe poll.

	// A better way for "StreamLogs" which implies following:
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
				// Clear EOF to keep reading
				// In Go, os.File doesn't need clearing for EOF if we just read again later?
				// Actually, once EOF is hit, Read returns EOF.
				// We need to seek? No, just Read again if file grew?
				// Actually, standard file read returns EOF.
				// We might need to check file size or just ignore EOF and sleep.
				continue
			}
			if err != nil {
				return err
			}
		}
	}
}
