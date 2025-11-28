package main

import (
	"context"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/config"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
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
	var queue acheron.Queue
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		redisDB := 0
		if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
			if db, err := strconv.Atoi(dbStr); err == nil {
				redisDB = db
			}
		}
		redisKey := os.Getenv("REDIS_QUEUE_KEY")
		if redisKey == "" {
			redisKey = "tartarus:queue"
		}

		rq, err := acheron.NewRedisQueue(redisAddr, redisDB, redisKey)
		if err != nil {
			logger.Error("Failed to initialize Redis queue", "error", err)
			os.Exit(1)
		}
		queue = rq
		logger.Info("Using Redis queue", "addr", redisAddr, "db", redisDB, "key", redisKey)
	} else {
		queue = acheron.NewMemoryQueue()
		logger.Info("Using in-memory queue")
	}
	registry := hades.NewMemoryRegistry()
	store, err := erebus.NewLocalStore(cfg.SnapshotPath)
	if err != nil {
		logger.Error("Failed to initialize store", "error", err)
		os.Exit(1)
	}
	_ = store // Silence unused variable error
	hermesLogger := hermes.NewSlogAdapter()
	var runtime tartarus.SandboxRuntime

	fcKernel := os.Getenv("FC_KERNEL_IMAGE")
	fcRootFS := os.Getenv("FC_ROOTFS_BASE")
	fcSocketDir := os.Getenv("FC_SOCKET_DIR")
	if fcSocketDir == "" {
		fcSocketDir = "/run/firecracker"
	}

	if fcKernel != "" && fcRootFS != "" {
		logger.Info("Initializing Firecracker Runtime", "kernel", fcKernel, "rootfs", fcRootFS)
		runtime = tartarus.NewFirecrackerRuntime(logger, fcSocketDir, fcKernel, fcRootFS)
	} else {
		logger.Info("Initializing Mock Runtime (Firecracker config missing)")
		runtime = tartarus.NewMockRuntime(logger)
	}
	metrics := hermes.NewNoopMetrics()

	// Mocks for dependencies not yet implemented
	nyxManager := &mockNyx{}
	lethePool := &mockLethe{}
	styxGateway := &mockStyx{}
	cocytusSink := &mockCocytus{}
	fury := erinyes.NewPollFury(runtime, hermesLogger, metrics, 1*time.Second)
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
				// Collect real system metrics
				vmStat, err := mem.VirtualMemory()
				if err != nil {
					logger.Error("Failed to get memory stats", "error", err)
					continue
				}

				cpuCount, err := cpu.Counts(true) // logical cores
				if err != nil {
					logger.Error("Failed to get CPU count", "error", err)
					continue
				}

				// Build resource capacity
				// Total memory in MB
				totalMemMB := domain.Megabytes(vmStat.Total / 1024 / 1024)
				// Total CPU in milliCPU (1 core = 1000 milliCPU)
				totalCPU := domain.MilliCPU(cpuCount * 1000)

				// Build heartbeat payload
				payload := hades.HeartbeatPayload{
					Node: domain.NodeInfo{
						ID:      agent.NodeID,
						Address: "localhost", // In production, this would be actual node address
						Labels:  map[string]string{"region": cfg.Region},
						Capacity: domain.ResourceCapacity{
							CPU: totalCPU,
							Mem: totalMemMB,
							GPU: 0,
						},
					},
					Load: domain.ResourceCapacity{
						// For now, report zero allocation
						// In a real system, the agent would track actual allocations
						CPU: 0,
						Mem: 0,
						GPU: 0,
					},
					Time: time.Now(),
				}

				// Send heartbeat to registry
				if err := registry.UpdateHeartbeat(ctx, payload); err != nil {
					logger.Error("Failed to send heartbeat", "error", err)
				} else {
					logger.Info("Heartbeat sent",
						"node_id", agent.NodeID,
						"total_mem_mb", totalMemMB,
						"total_cpu_milli", totalCPU)
				}
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
