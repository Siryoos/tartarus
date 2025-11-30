package tartarus

import (
	"context"
	"io"
	"net/netip"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// SandboxRuntime is the abstraction implemented by the MicroVM backend.
// Hecatoncheir Agent depends on this and does not care about Firecracker vs other VMM.

type SandboxRuntime interface {
	Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error)
	Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error)
	List(ctx context.Context) ([]domain.SandboxRun, error)
	Kill(ctx context.Context, id domain.SandboxID) error
	// Pause quiesces a running sandbox without destroying it.
	Pause(ctx context.Context, id domain.SandboxID) error
	// Resume unpauses a sandbox previously paused.
	Resume(ctx context.Context, id domain.SandboxID) error
	// CreateSnapshot captures the VM state into the provided mem/disk file paths.
	CreateSnapshot(ctx context.Context, id domain.SandboxID, memPath, diskPath string) error
	// Shutdown asks the sandbox to exit gracefully (CtrlAltDel for Firecracker).
	Shutdown(ctx context.Context, id domain.SandboxID) error
	// GetConfig returns the VMConfig and original request used to launch the sandbox.
	GetConfig(ctx context.Context, id domain.SandboxID) (VMConfig, *domain.SandboxRequest, error)
	StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error
	Allocation(ctx context.Context) (domain.ResourceCapacity, error)
	Wait(ctx context.Context, id domain.SandboxID) error
	Exec(ctx context.Context, id domain.SandboxID, cmd []string) error
}

// VMConfig captures low-level configuration required by the runtime.

type VMConfig struct {
	Snapshot  domain.SnapshotRef
	OverlayFS string // mount path for Lethe overlay
	TapDevice string // Styx-provided TAP name
	IP        netip.Addr
	Gateway   netip.Addr
	CIDR      netip.Prefix
	CPUs      int
	MemoryMB  int
}
