package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
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
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/lethe"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
	"github.com/tartarus-sandbox/tartarus/pkg/styx"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
	"github.com/tartarus-sandbox/tartarus/pkg/thanatos"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("Starting Hecatoncheir Agent", "region", cfg.Region)

	// Privileged check
	if os.Geteuid() != 0 {
		logger.Error("Fatal: Hecatoncheir Agent must run as root to access /dev/kvm and networking")
		os.Exit(1)
	}

	// Node Identity
	nodeID := domain.NodeID("node-" + cfg.Region + "-1")

	// Adapters
	metrics := hermes.NewLogMetrics()
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
		// Append NodeID to key for per-node queue
		redisKey = fmt.Sprintf("%s:%s", redisKey, nodeID)

		rq, err := acheron.NewRedisQueue(redisAddr, redisDB, redisKey, "acheron-workers", string(nodeID), false, metrics)
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

	var rdb *redis.Client
	if redisAddr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr: redisAddr,
			// DB: redisDB, // Use same DB
		})
	}

	var registry hades.Registry
	if cfg.RedisAddress != "" {
		r, err := hades.NewRedisRegistry(cfg.RedisAddress, cfg.RedisDB, cfg.RedisPass)
		if err != nil {
			logger.Error("Failed to initialize Redis registry", "error", err)
			os.Exit(1)
		}
		registry = r
		logger.Info("Using Redis registry", "addr", cfg.RedisAddress)
	} else {
		registry = hades.NewMemoryRegistry()
		logger.Info("Using in-memory registry")
	}

	// Erebus Store
	var store erebus.Store
	if cfg.S3Endpoint != "" || cfg.S3Region != "" {
		s3Store, err := erebus.NewS3Store(context.Background(), cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.SnapshotPath)
		if err != nil {
			logger.Error("Failed to initialize S3 store", "error", err)
			os.Exit(1)
		}
		store = s3Store
		logger.Info("Using S3 store", "bucket", cfg.S3Bucket)
	} else {
		localStore, err := erebus.NewLocalStore(cfg.SnapshotPath)
		if err != nil {
			logger.Error("Failed to initialize local store", "error", err)
			os.Exit(1)
		}
		store = localStore
		logger.Info("Using local store", "path", cfg.SnapshotPath)
	}

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

	// Styx Host Gateway
	bridgeName := "tartarus0"
	networkCIDR := os.Getenv("NETWORK_CIDR")
	if networkCIDR == "" {
		networkCIDR = "10.200.0.0/16"
	}

	prefix, err := netip.ParsePrefix(networkCIDR)
	if err != nil {
		logger.Error("Invalid network CIDR", "cidr", networkCIDR, "error", err)
		os.Exit(1)
	}

	styxGateway, err := styx.NewHostGateway(bridgeName, prefix)
	if err != nil {
		logger.Error("Failed to initialize Styx Host Gateway", "error", err)
		os.Exit(1)
	}

	// Lethe File Overlay Pool
	lethePool, err := lethe.NewFileOverlayPool(os.TempDir(), hermesLogger)
	if err != nil {
		logger.Error("Failed to initialize Lethe File Overlay Pool", "error", err)
		os.Exit(1)
	}

	// Nyx Local Manager
	nyxManager, err := nyx.NewLocalManager(store, cfg.SnapshotPath, hermesLogger)
	if err != nil {
		logger.Error("Failed to initialize Nyx Local Manager", "error", err)
		os.Exit(1)
	}

	// Cocytus Log Sink
	cocytusSink := cocytus.NewLogSink(logger)

	// Fury Watchdog
	fury := erinyes.NewPollFury(runtime, hermesLogger, metrics, 1*time.Second)

	// Judges
	judgeChain := &judges.Chain{}

	// Hypnos (Sleep Manager)
	hypnosManager := hypnos.NewManager(runtime, store, os.TempDir())

	// Thanatos (Termination Handler)
	thanatosHandler := thanatos.NewHandler(runtime, hypnosManager)

	// Control Listener
	var controlListener hecatoncheir.ControlListener
	if rdb != nil {
		controlListener = hecatoncheir.NewRedisControlListener(rdb, nodeID)
		logger.Info("Enabled Redis control listener")
	}

	agent := &hecatoncheir.Agent{
		NodeID:     nodeID,
		Runtime:    runtime,
		Nyx:        nyxManager,
		Lethe:      lethePool,
		Styx:       styxGateway,
		Judges:     judgeChain,
		Furies:     fury,
		Hypnos:     hypnosManager,
		Thanatos:   thanatosHandler,
		Queue:      queue,
		Registry:   registry,
		DeadLetter: cocytusSink,
		Control:    controlListener,
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

				// Get active sandboxes
				activeSandboxes, err := runtime.List(ctx)
				if err != nil {
					logger.Error("Failed to list active sandboxes", "error", err)
					activeSandboxes = []domain.SandboxRun{}
				}

				// Get actual allocation from Runtime
				allocated, err := runtime.Allocation(ctx)
				if err != nil {
					logger.Error("Failed to get allocation stats", "error", err)
					// Fallback to zero if failed, or continue?
					// Just log and keep allocated at 0 default
				}

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
					Load:            allocated,
					ActiveSandboxes: activeSandboxes,
					Time:            time.Now(),
				}

				// Send heartbeat to registry
				if err := registry.UpdateHeartbeat(ctx, payload); err != nil {
					logger.Error("Failed to send heartbeat", "error", err)
				} else {
					logger.Info("Heartbeat sent",
						"node_id", agent.NodeID,
						"allocated_cpu", allocated.CPU,
						"allocated_mem", allocated.Mem)
				}
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down agent...")
}
