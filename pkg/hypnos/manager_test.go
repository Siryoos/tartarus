package hypnos

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

func TestSleepAndWake(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	runtime := tartarus.NewMockRuntime(logger)

	storeDir := t.TempDir()
	store, err := erebus.NewLocalStore(storeDir)
	require.NoError(t, err)

	manager := NewManager(runtime, store, t.TempDir())

	req := &domain.SandboxRequest{
		ID:       "sandbox-1",
		Template: "tpl-1",
		Resources: domain.ResourceSpec{
			CPU: 1,
			Mem: 128,
		},
	}
	cfg := tartarus.VMConfig{
		OverlayFS: "/tmp/ov-1",
		CPUs:      1,
		MemoryMB:  128,
	}

	_, err = runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	record, err := manager.Sleep(ctx, req.ID, nil)
	require.NoError(t, err)
	require.Equal(t, req.ID, record.SandboxID)
	require.True(t, manager.IsSleeping(req.ID))

	// Snapshot files should exist in the store.
	_, err = os.Stat(filepath.Join(storeDir, record.SnapshotKey+".mem.gz"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(storeDir, record.SnapshotKey+".disk"))
	require.NoError(t, err)

	run, err := manager.Wake(ctx, req.ID)
	require.NoError(t, err)
	require.Equal(t, req.ID, run.ID)
	require.False(t, manager.IsSleeping(req.ID))
}

func TestWakeWithoutSleepFails(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	store, err := erebus.NewLocalStore(t.TempDir())
	require.NoError(t, err)
	manager := NewManager(runtime, store, t.TempDir())

	_, err = manager.Wake(ctx, "missing")
	require.Error(t, err)
}
