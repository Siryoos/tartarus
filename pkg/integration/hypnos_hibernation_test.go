package integration

import (
	"compress/gzip"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

func TestHypnosHibernateAndWake(t *testing.T) {
	// Setup
	ctx := context.Background()
	metrics := hermes.NewLogMetrics()

	// Mock Runtime
	runtime := tartarus.NewMockRuntime(slog.Default())
	runtime.SetStartDuration(1 * time.Millisecond)

	// Local Store
	tmpDir := t.TempDir()
	store, err := erebus.NewLocalStore(filepath.Join(tmpDir, "store"))
	require.NoError(t, err)

	// Hypnos Manager
	stagingDir := filepath.Join(tmpDir, "staging")
	err = os.MkdirAll(stagingDir, 0755)
	require.NoError(t, err)
	manager := hypnos.NewManager(runtime, store, stagingDir)
	manager.Metrics = metrics

	// 1. Launch a Sandbox
	req := &domain.SandboxRequest{
		ID:       "test-sandbox",
		Template: "test-template",
		Resources: domain.ResourceSpec{
			CPU: 100,
			Mem: 128,
		},
	}
	cfg := tartarus.VMConfig{
		TapDevice: "tap0",
	}
	run, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusRunning, run.Status)

	// 2. Hibernate (Sleep)
	sleepOpts := &hypnos.SleepOptions{GracefulShutdown: true}
	record, err := manager.Sleep(ctx, req.ID, sleepOpts)
	require.NoError(t, err)
	assert.NotNil(t, record)
	assert.Equal(t, req.ID, record.SandboxID)

	// Verify Runtime State (Should be killed/gone from runtime perspective after sleep)
	_, err = runtime.Inspect(ctx, req.ID)
	assert.Error(t, err, "Sandbox should be gone from runtime after sleep")

	// Verify Snapshot Storage
	// Memory should be compressed
	memKey := record.SnapshotKey + ".mem.gz"
	exists, err := store.Exists(ctx, memKey)
	require.NoError(t, err)
	assert.True(t, exists, "Compressed memory snapshot should exist")

	// Verify Compression
	memReader, err := store.Get(ctx, memKey)
	require.NoError(t, err)
	defer memReader.Close()

	// Check gzip header
	gzipReader, err := gzip.NewReader(memReader)
	require.NoError(t, err, "Snapshot should be valid gzip")
	gzipReader.Close()

	// Disk should exist
	diskKey := record.SnapshotKey + ".disk"
	exists, err = store.Exists(ctx, diskKey)
	require.NoError(t, err)
	assert.True(t, exists, "Disk snapshot should exist")

	// 3. Wake
	start := time.Now()
	wakeRun, err := manager.Wake(ctx, req.ID)
	duration := time.Since(start)
	t.Logf("Wake duration: %v", duration)

	require.NoError(t, err)
	assert.Less(t, duration, 100*time.Millisecond, "Wake latency should be < 100ms")
	assert.NotNil(t, wakeRun)
	assert.Equal(t, req.ID, wakeRun.ID)
	assert.Equal(t, domain.RunStatusRunning, wakeRun.Status)

	// Verify Runtime State
	inspected, err := runtime.Inspect(ctx, req.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusRunning, inspected.Status)
}
