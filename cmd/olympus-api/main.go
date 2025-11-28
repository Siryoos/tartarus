package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/config"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
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
	registry := hades.NewMemoryRegistry()
	store, err := erebus.NewLocalStore(cfg.SnapshotPath)
	if err != nil {
		logger.Error("Failed to initialize store", "error", err)
		os.Exit(1)
	}
	_ = store // Silence unused variable error
	scheduler := olympus.NewMemoryScheduler(logger)
	metrics := hermes.NewNoopMetrics()
	hermesLogger := hermes.NewSlogAdapter()

	// Policy repository
	policyRepo := themis.NewMemoryRepo()

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
			logger.Error("Failed to submit request", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "id": string(req.ID)})
	})

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
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
