package lethe

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
)

// FileOverlayPool implements Pool using simple file copies.
type FileOverlayPool struct {
	BaseDir string
	Logger  hermes.Logger
}

// NewFileOverlayPool creates a new file-based overlay pool.
func NewFileOverlayPool(baseDir string, logger hermes.Logger) (*FileOverlayPool, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to ensure base dir exists: %w", err)
	}
	return &FileOverlayPool{
		BaseDir: baseDir,
		Logger:  logger,
	}, nil
}

// Create creates a new overlay by copying the snapshot file.
func (p *FileOverlayPool) Create(ctx context.Context, snapshot *nyx.Snapshot) (*Overlay, error) {
	id := uuid.New().String()
	overlayPath := filepath.Join(p.BaseDir, fmt.Sprintf("%s.img", id))

	if p.Logger != nil {
		p.Logger.Info(ctx, "Creating overlay", map[string]any{
			"overlay_id":  id,
			"snapshot_id": snapshot.ID,
			"base_path":   snapshot.Path,
			"mount_path":  overlayPath,
		})
	}

	// Copy the snapshot file to the overlay path
	// Snapshot.Path is the base path (without extension), but we need the disk image.
	snapshotDiskPath := snapshot.Path + ".disk"
	if err := copyFile(snapshotDiskPath, overlayPath); err != nil {
		return nil, fmt.Errorf("failed to copy snapshot: %w", err)
	}

	return &Overlay{
		ID:              id,
		MountPath:       overlayPath,
		BackingSnapshot: snapshot.ID,
	}, nil
}

// Destroy removes the overlay file.
func (p *FileOverlayPool) Destroy(ctx context.Context, overlay *Overlay) error {
	if p.Logger != nil {
		p.Logger.Info(ctx, "Destroying overlay", map[string]any{
			"overlay_id": overlay.ID,
			"mount_path": overlay.MountPath,
		})
	}

	if err := os.Remove(overlay.MountPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove overlay file: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}
