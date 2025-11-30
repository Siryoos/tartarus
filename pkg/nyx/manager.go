package nyx

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Snapshot represents a prepared VM state on disk.

type Snapshot struct {
	ID        domain.SnapshotID `json:"id"`
	Template  domain.TemplateID `json:"template"`
	Path      string            `json:"path"`
	CreatedAt time.Time         `json:"created_at"`
	Metadata  map[string]string `json:"metadata"`
}

// Manager is Nyx: responsible for preparing and serving snapshots.

type Manager interface {
	// Prepare ensures there is at least one snapshot for the given template.
	// It may boot a template VM, run warmup, and persist a snapshot in Erebus.
	Prepare(ctx context.Context, tpl *domain.TemplateSpec) (*Snapshot, error)

	// GetSnapshot returns a ready snapshot for this template.
	GetSnapshot(ctx context.Context, tplID domain.TemplateID) (*Snapshot, error)

	// ListSnapshots returns all snapshots for a template.
	ListSnapshots(ctx context.Context, tplID domain.TemplateID) ([]*Snapshot, error)

	// Invalidate can be called when a template is updated or revoked.
	Invalidate(ctx context.Context, tplID domain.TemplateID) error

	// SaveSnapshot persists a snapshot from local paths to the store.
	SaveSnapshot(ctx context.Context, tplID domain.TemplateID, snapID domain.SnapshotID, memPath, diskPath string) (*Snapshot, error)

	// DeleteSnapshot removes a snapshot from the store and cache.
	DeleteSnapshot(ctx context.Context, tplID domain.TemplateID, snapID domain.SnapshotID) error
}
