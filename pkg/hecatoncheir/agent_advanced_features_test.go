package hecatoncheir

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
	"github.com/tartarus-sandbox/tartarus/pkg/thanatos"
)

func TestAgent_Integration_HibernateWake(t *testing.T) {
	// Setup
	logger := hermes.NewSlogAdapter()
	mockRuntime := new(MockRuntime)
	mockControl := new(MockControlListener)

	tmpDir, err := os.MkdirTemp("", "hypnos-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	store, _ := erebus.NewLocalStore(tmpDir)

	hypnosManager := hypnos.NewManager(mockRuntime, store, tmpDir)
	agent := &Agent{
		Runtime: mockRuntime,
		Hypnos:  hypnosManager,
		Control: mockControl,
		Logger:  logger,
	}

	sandboxID := domain.SandboxID("sandbox-sleep-1")

	// 1. HIBERNATE
	// Setup Runtime Expectations
	mockRuntime.On("GetConfig", mock.Anything, sandboxID).Return(
		tartarus.VMConfig{}, &domain.SandboxRequest{ID: sandboxID}, nil,
	)
	mockRuntime.On("Pause", mock.Anything, sandboxID).Return(nil)
	// We must simulate snapshot creation by writing files when CreateSnapshot is called
	mockRuntime.On("CreateSnapshot", mock.Anything, sandboxID, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		os.WriteFile(args.String(2), []byte("mem"), 0600)
		os.WriteFile(args.String(3), []byte("disk"), 0600)
	}).Return(nil)
	mockRuntime.On("Shutdown", mock.Anything, sandboxID).Return(nil)
	mockRuntime.On("Kill", mock.Anything, sandboxID).Return(nil)

	// Send Message
	ch := make(chan ControlMessage)
	go func() {
		ch <- ControlMessage{Type: ControlMessageHibernate, SandboxID: sandboxID}
		close(ch)
	}()

	agent.controlLoop(context.Background(), ch)

	// Verify
	mockRuntime.AssertCalled(t, "Pause", mock.Anything, sandboxID)
	mockRuntime.AssertCalled(t, "CreateSnapshot", mock.Anything, sandboxID, mock.Anything, mock.Anything)

	// 2. WAKE
	// Use a new channel for Wake
	// Hypnos should now have a record of the sleeping sandbox
	require.True(t, hypnosManager.IsSleeping(sandboxID))

	mockRuntime.On("Launch", mock.Anything, mock.Anything, mock.Anything).Return(&domain.SandboxRun{ID: sandboxID}, nil)

	ch2 := make(chan ControlMessage)
	go func() {
		ch2 <- ControlMessage{Type: ControlMessageWake, SandboxID: sandboxID}
		close(ch2)
	}()

	agent.controlLoop(context.Background(), ch2)

	mockRuntime.AssertCalled(t, "Launch", mock.Anything, mock.Anything, mock.Anything)
	require.False(t, hypnosManager.IsSleeping(sandboxID))
}

func TestAgent_Integration_Terminate(t *testing.T) {
	// Setup
	logger := hermes.NewSlogAdapter()
	mockRuntime := new(MockRuntime)
	mockControl := new(MockControlListener)

	tmpDir, err := os.MkdirTemp("", "thanatos-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Thanatos needs Hypnos (optional but passed in agent)
	// Pass nil to simplify if we don't test checkpointing here
	thanatosHandler := thanatos.NewHandler(mockRuntime, nil)

	agent := &Agent{
		Runtime:  mockRuntime,
		Thanatos: thanatosHandler,
		Control:  mockControl,
		Logger:   logger,
	}

	sandboxID := domain.SandboxID("sandbox-term-1")

	// Expect Shutdown and Wait
	mockRuntime.On("Shutdown", mock.Anything, sandboxID).Return(nil)
	mockRuntime.On("Wait", mock.Anything, sandboxID).Return(nil)
	mockRuntime.On("Inspect", mock.Anything, sandboxID).Return(&domain.SandboxRun{ExitCode: pointerToInt(0)}, nil)

	ch := make(chan ControlMessage)
	go func() {
		// Args: grace_period
		ch <- ControlMessage{Type: ControlMessageTerminate, SandboxID: sandboxID, Args: []string{"100ms"}}
		close(ch)
	}()

	agent.controlLoop(context.Background(), ch)

	mockRuntime.AssertCalled(t, "Shutdown", mock.Anything, sandboxID)
	mockRuntime.AssertCalled(t, "Wait", mock.Anything, sandboxID)
}

func pointerToInt(i int) *int { return &i }
