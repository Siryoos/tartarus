package main

import (
	"context"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/config"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/hecatoncheir"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/lethe"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
	"github.com/tartarus-sandbox/tartarus/pkg/styx"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("Starting Hecatoncheir Agent", "region", cfg.Region)

	// Adapters
	queue := acheron.NewMemoryQueue()
	store, err := erebus.NewLocalStore(cfg.SnapshotPath)
	if err != nil {
		logger.Error("Failed to initialize store", "error", err)
		os.Exit(1)
	}
	_ = store // Silence unused variable error
	hermesLogger := hermes.NewSlogAdapter()
	runtime := tartarus.NewMockRuntime(logger)
	metrics := hermes.NewNoopMetrics()

	// Mocks for dependencies not yet implemented
	nyxManager := &mockNyx{}
	lethePool := &mockLethe{}
	styxGateway := &mockStyx{}
	cocytusSink := &mockCocytus{}
	fury := &mockFury{}
	judgeChain := &judges.Chain{}

	agent := &hecatoncheir.Agent{
		NodeID:     domain.NodeID("node-" + cfg.Region + "-1"),
		Runtime:    runtime,
		Nyx:        nyxManager,
		Lethe:      lethePool,
		Styx:       styxGateway,
		Judges:     judgeChain,
		Furies:     fury,
		Queue:      queue,
		DeadLetter: cocytusSink,
		Metrics:    metrics,
		Logger:     hermesLogger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Agent Loop
	go func() {
		if err := agent.Run(ctx); err != nil {
			logger.Error("Agent loop failed", "error", err)
		}
	}()

	// Heartbeat Ticker
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Info("Sending heartbeat...")
				// In a real app, we'd call Hades client here
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down agent...")
}

// Mocks

type mockNyx struct{}

func (m *mockNyx) Prepare(ctx context.Context, tpl *domain.TemplateSpec) (*nyx.Snapshot, error) {
	return &nyx.Snapshot{}, nil
}
func (m *mockNyx) GetSnapshot(ctx context.Context, tplID domain.TemplateID) (*nyx.Snapshot, error) {
	return &nyx.Snapshot{}, nil
}
func (m *mockNyx) ListSnapshots(ctx context.Context, tplID domain.TemplateID) ([]*nyx.Snapshot, error) {
	return nil, nil
}
func (m *mockNyx) Invalidate(ctx context.Context, tplID domain.TemplateID) error { return nil }

type mockLethe struct{}

func (m *mockLethe) Create(ctx context.Context, snapshot *nyx.Snapshot) (*lethe.Overlay, error) {
	return &lethe.Overlay{}, nil
}
func (m *mockLethe) Destroy(ctx context.Context, overlay *lethe.Overlay) error { return nil }

type mockStyx struct{}

func (m *mockStyx) Attach(ctx context.Context, sandboxID domain.SandboxID, contract *styx.Contract) (string, netip.Addr, error) {
	return "tap0", netip.Addr{}, nil
}
func (m *mockStyx) Detach(ctx context.Context, sandboxID domain.SandboxID) error { return nil }

type mockCocytus struct{}

func (m *mockCocytus) Write(ctx context.Context, rec *cocytus.Record) error { return nil }

type mockFury struct{}

func (m *mockFury) Arm(ctx context.Context, run *domain.SandboxRun, policy *erinyes.PolicySnapshot) error {
	return nil
}
func (m *mockFury) Disarm(ctx context.Context, runID domain.SandboxID) error { return nil }
