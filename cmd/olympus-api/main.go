package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/config"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
	"github.com/tartarus-sandbox/tartarus/pkg/olympus"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("Starting Olympus API", "port", cfg.Port)

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

	var registry hades.Registry
	if cfg.RedisAddress != "" {
		rr, err := hades.NewRedisRegistry(cfg.RedisAddress, cfg.RedisDB, cfg.RedisPass)
		if err != nil {
			logger.Error("Failed to initialize Redis registry", "error", err)
			os.Exit(1)
		}
		registry = rr
		logger.Info("Using Redis registry", "addr", cfg.RedisAddress)
	} else {
		registry = hades.NewMemoryRegistry()
		logger.Info("Using in-memory registry")
	}

	var store erebus.Store
	if cfg.S3Endpoint != "" || cfg.S3Region != "" {
		// If S3 config is present, use S3Store
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
	_ = store // Silence unused variable error
	_ = store // Silence unused variable error
	metrics := hermes.NewNoopMetrics()
	hermesLogger := hermes.NewSlogAdapter()
	scheduler := moirai.NewLeastLoadedScheduler(hermesLogger)

	// Policy repository
	policyRepo := themis.NewMemoryRepo()

	// Control Plane
	var control olympus.ControlPlane
	if redisAddr != "" {
		rdb := redis.NewClient(&redis.Options{
			Addr: redisAddr,
			// DB: redisDB,
		})
		control = olympus.NewRedisControlPlane(rdb)
		logger.Info("Using Redis control plane")
	} else {
		control = &olympus.NoopControlPlane{}
		logger.Info("Using Noop control plane")
	}

	// Judges
	resourceJudge := judges.NewResourceJudge(policyRepo, hermesLogger)
	networkJudge := judges.NewNetworkJudge([]netip.Prefix{}, hermesLogger)
	judgeChain := &judges.Chain{
		Pre: []judges.PreJudge{resourceJudge, networkJudge},
	}

	manager := &olympus.Manager{
		Queue:     queue,
		Hades:     registry,
		Policies:  policyRepo,
		Judges:    judgeChain,
		Scheduler: scheduler,
		Control:   control,
		Metrics:   metrics,
		Logger:    hermesLogger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req domain.SandboxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := manager.Submit(r.Context(), &req); err != nil {
			if errors.Is(err, olympus.ErrPolicyRejected) {
				logger.Warn("Request rejected by policy", "error", err)
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			logger.Error("Failed to submit request", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "id": string(req.ID)})
	})

	mux.HandleFunc("/sandboxes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		runs, err := manager.ListSandboxes(r.Context())
		if err != nil {
			logger.Error("Failed to list sandboxes", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(runs)
	})

	mux.HandleFunc("/sandboxes/", func(w http.ResponseWriter, r *http.Request) {
		id := domain.SandboxID(r.URL.Path[len("/sandboxes/"):])
		if id == "" {
			http.Error(w, "Missing sandbox ID", http.StatusBadRequest)
			return
		}

		if r.Method == http.MethodDelete {
			if err := manager.KillSandbox(r.Context(), id); err != nil {
				logger.Error("Failed to kill sandbox", "id", id, "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		} else if r.Method == http.MethodGet && r.URL.Query().Get("action") == "logs" {
			// Stream logs
			// Check if we should stream
			// Path: /sandboxes/{id}?action=logs
			// Or maybe /sandboxes/{id}/logs is better but mux doesn't support wildcards easily without stripping.
			// Let's stick to query param or check suffix if I parse path manually.
			// The handler is registered on /sandboxes/, so it matches /sandboxes/foo/logs if I parse it.
		}
	})

	// Explicit handler for logs to make it cleaner?
	// But /sandboxes/ overlaps.
	// Let's use a specific prefix for logs or handle it in the generic handler.
	// Let's try to handle /sandboxes/{id}/logs by checking suffix.
	mux.HandleFunc("/sandboxes/logs/", func(w http.ResponseWriter, r *http.Request) {
		// /sandboxes/logs/{id}
		id := domain.SandboxID(r.URL.Path[len("/sandboxes/logs/"):])
		if id == "" {
			http.Error(w, "Missing sandbox ID", http.StatusBadRequest)
			return
		}

		// Set headers for streaming
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if err := manager.StreamLogs(r.Context(), id, w); err != nil {
			logger.Error("Log streaming failed", "id", id, "error", err)
			// Cannot write error status if we already started writing?
			// But StreamLogs writes to w.
		}
	})

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: olympus.AuthMiddleware(logger, mux),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}
	logger.Info("Server exited")
}
