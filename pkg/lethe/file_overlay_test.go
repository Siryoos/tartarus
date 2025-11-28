package lethe

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
)

func TestFileOverlayPool(t *testing.T) {
	// Setup temporary directory for tests
	tmpDir, err := os.MkdirTemp("", "lethe-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	baseDir := filepath.Join(tmpDir, "overlays")
	snapshotDir := filepath.Join(tmpDir, "snapshots")
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		t.Fatalf("failed to create snapshot dir: %v", err)
	}

	// Create a dummy snapshot file
	snapshotPath := filepath.Join(snapshotDir, "base.img")
	initialContent := []byte("original content")
	if err := os.WriteFile(snapshotPath, initialContent, 0644); err != nil {
		t.Fatalf("failed to write snapshot file: %v", err)
	}

	// Create the pool
	pool, err := NewFileOverlayPool(baseDir, nil)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	// Test Create
	ctx := context.Background()
	snapshot := &nyx.Snapshot{
		ID:   "snap-1",
		Path: snapshotPath,
	}

	overlay, err := pool.Create(ctx, snapshot)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if overlay.ID == "" {
		t.Error("Overlay ID is empty")
	}
	if overlay.BackingSnapshot != snapshot.ID {
		t.Errorf("expected backing snapshot %s, got %s", snapshot.ID, overlay.BackingSnapshot)
	}
	if overlay.MountPath == "" {
		t.Error("Overlay MountPath is empty")
	}

	// Verify overlay file exists
	if _, err := os.Stat(overlay.MountPath); os.IsNotExist(err) {
		t.Error("Overlay file was not created")
	}

	// Verify content matches
	content, err := os.ReadFile(overlay.MountPath)
	if err != nil {
		t.Fatalf("failed to read overlay file: %v", err)
	}
	if string(content) != string(initialContent) {
		t.Errorf("expected content %q, got %q", initialContent, content)
	}

	// Modify overlay file (simulate VM writing)
	newContent := []byte("modified content")
	if err := os.WriteFile(overlay.MountPath, newContent, 0644); err != nil {
		t.Fatalf("failed to write to overlay file: %v", err)
	}

	// Verify snapshot is untouched
	snapContent, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("failed to read snapshot file: %v", err)
	}
	if string(snapContent) != string(initialContent) {
		t.Errorf("Snapshot was modified! Expected %q, got %q", initialContent, snapContent)
	}

	// Test Destroy
	if err := pool.Destroy(ctx, overlay); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}

	// Verify overlay file is gone
	if _, err := os.Stat(overlay.MountPath); !os.IsNotExist(err) {
		t.Error("Overlay file was not removed")
	}

	// Verify snapshot is still there
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Error("Snapshot file was removed!")
	}

	// Test Destroy again (idempotency)
	if err := pool.Destroy(ctx, overlay); err != nil {
		t.Errorf("Destroy failed on second call: %v", err)
	}
}
