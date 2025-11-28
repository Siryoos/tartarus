package lethe

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
)

// Overlay describes an ephemeral writable filesystem attached to a snapshot.

type Overlay struct {
	ID              string            `json:"id"`
	MountPath       string            `json:"mount_path"`
	BackingSnapshot domain.SnapshotID `json:"backing_snapshot"`
}

// Pool is Lethe: creates and forgets overlays.

type Pool interface {
	Create(ctx context.Context, snapshot *nyx.Snapshot) (*Overlay, error)
	Destroy(ctx context.Context, overlay *Overlay) error
}
