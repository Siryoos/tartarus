//go:build !linux
// +build !linux

package nyx

import (
	"context"
	"fmt"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

type LocalManager struct {
}

func NewLocalManager(store erebus.Store, ociBuilder *erebus.OCIBuilder, snapshotDir string, logger hermes.Logger) (*LocalManager, error) {
	return &LocalManager{}, nil
}

func (m *LocalManager) Prepare(ctx context.Context, tpl *domain.TemplateSpec) (*Snapshot, error) {
	return nil, fmt.Errorf("Nyx LocalManager not supported on non-Linux platforms")
}

func (m *LocalManager) GetSnapshot(ctx context.Context, tplID domain.TemplateID) (*Snapshot, error) {
	return nil, fmt.Errorf("Nyx LocalManager not supported on non-Linux platforms")
}

func (m *LocalManager) ListSnapshots(ctx context.Context, tplID domain.TemplateID) ([]*Snapshot, error) {
	return nil, fmt.Errorf("Nyx LocalManager not supported on non-Linux platforms")
}

func (m *LocalManager) Invalidate(ctx context.Context, tplID domain.TemplateID) error {
	return fmt.Errorf("Nyx LocalManager not supported on non-Linux platforms")
}

func (m *LocalManager) SaveSnapshot(ctx context.Context, tplID domain.TemplateID, snapID domain.SnapshotID, memPath, diskPath string) (*Snapshot, error) {
	return nil, fmt.Errorf("Nyx LocalManager not supported on non-Linux platforms")
}

func (m *LocalManager) DeleteSnapshot(ctx context.Context, tplID domain.TemplateID, snapID domain.SnapshotID) error {
	return fmt.Errorf("Nyx LocalManager not supported on non-Linux platforms")
}
