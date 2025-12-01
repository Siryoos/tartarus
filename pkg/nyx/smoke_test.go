//go:build linux

package nyx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// MockSmokeSnapshotMachine implements SnapshotMachine for smoke tests
type MockSmokeSnapshotMachine struct{}

func (m *MockSmokeSnapshotMachine) CreateSnapshot(ctx context.Context, memFilePath, snapshotPath string, opts ...firecracker.CreateSnapshotOpt) error {
	if err := os.WriteFile(memFilePath, []byte("smoke-mem"), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(snapshotPath, []byte("smoke-disk"), 0644); err != nil {
		return err
	}
	return nil
}

func (m *MockSmokeSnapshotMachine) StopVMM() error {
	return nil
}

func TestSmoke_WarmupFlow(t *testing.T) {
	// 1. Setup Environment
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	snapDir := filepath.Join(tmpDir, "snapshots")

	store, err := erebus.NewLocalStore(storeDir)
	assert.NoError(t, err)

	logger := hermes.NewSlogAdapter()
	mgr, err := NewLocalManager(store, nil, snapDir, logger)
	assert.NoError(t, err)

	// Mock VM Launcher
	mgr.vmLauncher = func(ctx context.Context, tpl *domain.TemplateSpec, rootfsPath, socketPath string) (SnapshotMachine, error) {
		return &MockSmokeSnapshotMachine{}, nil
	}

	// 2. Define Template
	tpl := &domain.TemplateSpec{
		ID:          "smoke-tpl",
		BaseImage:   "ubuntu:22.04",
		KernelImage: "vmlinux-5.10",
		Resources: domain.ResourceSpec{
			CPU: 2000,
			Mem: 512,
		},
		WarmupCommand: []string{"echo", "hello"},
	}

	// 3. Execute Prepare (Warmup -> Snapshot)
	ctx := context.Background()
	snap, err := mgr.Prepare(ctx, tpl)
	assert.NoError(t, err)
	assert.NotNil(t, snap)
	assert.NotEmpty(t, snap.ID)
	assert.Equal(t, tpl.ID, snap.Template)

	// Verify Metadata
	assert.Equal(t, "ubuntu:22.04", snap.Metadata["source_image"])
	assert.Equal(t, "vmlinux-5.10", snap.Metadata["kernel_image"])
	assert.Equal(t, "2", snap.Metadata["cpu_count"])
	assert.Equal(t, "512", snap.Metadata["mem_size_mb"])

	// 4. Verify Persistence (Simulate restart by creating new manager)
	mgr2, err := NewLocalManager(store, nil, t.TempDir(), logger) // New local cache
	assert.NoError(t, err)
	// No vmLauncher needed for read-through

	// 5. Execute GetSnapshot (Read-through)
	snap2, err := mgr2.GetSnapshot(ctx, tpl.ID)
	assert.NoError(t, err)
	assert.NotNil(t, snap2)
	assert.Equal(t, snap.ID, snap2.ID)

	// Verify Metadata persisted
	assert.Equal(t, "ubuntu:22.04", snap2.Metadata["source_image"])
	assert.Equal(t, "vmlinux-5.10", snap2.Metadata["kernel_image"])
	assert.Equal(t, "2", snap2.Metadata["cpu_count"])
	assert.Equal(t, "512", snap2.Metadata["mem_size_mb"])
	assert.WithinDuration(t, snap.CreatedAt, snap2.CreatedAt, 1*time.Second)
}
