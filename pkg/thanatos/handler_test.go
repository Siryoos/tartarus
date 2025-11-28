package thanatos

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

func TestTerminateGracefully(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	req := &domain.SandboxRequest{
		ID:       "graceful-1",
		Template: "tpl",
	}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	handler := NewHandler(runtime, nil)
	result, err := handler.Terminate(ctx, req.ID, Options{GracePeriod: 100 * time.Millisecond})
	require.NoError(t, err)
	require.Equal(t, PhaseCompleted, result.Phase)
}

func TestTerminateWithTimeoutKill(t *testing.T) {
	ctx := context.Background()
	runtime := tartarus.NewMockRuntime(slog.Default())
	runtime.ShutdownDelay = 200 * time.Millisecond

	req := &domain.SandboxRequest{ID: "slow-1", Template: "tpl"}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err := runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	handler := NewHandler(runtime, nil)
	result, err := handler.Terminate(ctx, req.ID, Options{GracePeriod: 10 * time.Millisecond})
	require.Error(t, err)
	require.Equal(t, PhaseKilled, result.Phase)
}

func TestTerminateWithCheckpoint(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	runtime := tartarus.NewMockRuntime(logger)
	store, err := erebus.NewLocalStore(t.TempDir())
	require.NoError(t, err)
	sleepManager := hypnos.NewManager(runtime, store, t.TempDir())

	req := &domain.SandboxRequest{ID: "checkpoint-1", Template: "tpl"}
	cfg := tartarus.VMConfig{CPUs: 1, MemoryMB: 64}
	_, err = runtime.Launch(ctx, req, cfg)
	require.NoError(t, err)

	handler := NewHandler(runtime, sleepManager)
	result, err := handler.Terminate(ctx, req.ID, Options{CreateCheckpoint: true})
	require.NoError(t, err)
	require.Equal(t, PhaseCheckpointed, result.Phase)
	require.NotEmpty(t, result.Checkpoint)
}
