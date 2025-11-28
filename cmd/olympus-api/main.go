package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("Starting Olympus API", "port", cfg.Port)

	// Adapters
	queue := acheron.NewMemoryQueue()
	registry := hades.NewMemoryRegistry()
	store, err := erebus.NewLocalStore(cfg.SnapshotPath)
	if err != nil {
		logger.Error("Failed to initialize store", "error", err)
		os.Exit(1)
	}
	_ = store // Silence unused variable error
	scheduler := olympus.NewMemoryScheduler()
	metrics := hermes.NewNoopMetrics()
	hermesLogger := hermes.NewSlogAdapter()

	// Mocks for Policy and Judges (since not implemented yet)
	// We need a simple implementation for Themis Repository
	// For now, we'll pass nil or a mock if we had one.
	// Since Manager expects interfaces, we need to satisfy them.
	// Let's create simple inline mocks or just use nil if the code handles it (it probably doesn't).
	// We'll create a simple mock for Themis here.

	policyRepo := &mockPolicyRepo{}
	judgeChain := &judges.Chain{} // Empty chain is valid

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

		// In a real app, we'd call manager.Submit(r.Context(), &req)
		// But Manager.Submit is empty in the scaffold.
		// We'll simulate it here or assume Manager.Submit will be implemented later.
		// For now, let's just enqueue it directly to prove the wiring works,
		// or call Submit if we trust it does something (it returns nil currently).

		if err := manager.Submit(r.Context(), &req); err != nil {
			http.Error(w, "Failed to submit request", http.StatusInternalServerError)
			return
		}

		// Since Submit is empty, let's manually enqueue to make the agent see it
		// This is a temporary hack until Manager.Submit is implemented properly
		if err := queue.Enqueue(r.Context(), &req); err != nil {
			http.Error(w, "Failed to enqueue", http.StatusInternalServerError)
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

// Mock Policy Repo
type mockPolicyRepo struct{}

func (m *mockPolicyRepo) GetPolicy(ctx context.Context, tplID domain.TemplateID) (*domain.SandboxPolicy, error) {
	return &domain.SandboxPolicy{}, nil
}
func (m *mockPolicyRepo) UpsertPolicy(ctx context.Context, p *domain.SandboxPolicy) error { return nil }
func (m *mockPolicyRepo) ListPolicies(ctx context.Context) ([]*domain.SandboxPolicy, error) {
	return nil, nil
}
