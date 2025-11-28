package tartarus

import (
	"context"
	"io"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// SandboxRuntime is the abstraction implemented by the MicroVM backend.
// Hecatoncheir Agent depends on this and does not care about Firecracker vs other VMM.

type SandboxRuntime interface {
	Launch(ctx context.Context, req *domain.SandboxRequest, cfg VMConfig) (*domain.SandboxRun, error)
	Inspect(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error)
	List(ctx context.Context) ([]domain.SandboxRun, error)
	Kill(ctx context.Context, id domain.SandboxID) error
	StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer) error
}

// VMConfig captures low-level configuration required by the runtime.

type VMConfig struct {
	Snapshot  domain.SnapshotRef
	OverlayFS string // mount path for Lethe overlay
	TapDevice string // Styx-provided TAP name
	CPUs      int
	MemoryMB  int
}
