package erinyes

import (
	"context"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// PollFury is a poll-based implementation of the Fury interface.
// It periodically checks running sandboxes and enforces runtime and memory limits.
type PollFury struct {
	Runtime  tartarus.SandboxRuntime
	Logger   hermes.Logger
	Metrics  hermes.Metrics
	Interval time.Duration

	mu     sync.Mutex
	active map[domain.SandboxID]context.CancelFunc
}

// NewPollFury creates a new PollFury instance.
func NewPollFury(runtime tartarus.SandboxRuntime, logger hermes.Logger, metrics hermes.Metrics, interval time.Duration) *PollFury {
	return &PollFury{
		Runtime:  runtime,
		Logger:   logger,
		Metrics:  metrics,
		Interval: interval,
		active:   make(map[domain.SandboxID]context.CancelFunc),
	}
}

// Arm starts a watcher for the given sandbox run.
// If policy.KillOnBreach is false, this is a no-op.
func (p *PollFury) Arm(ctx context.Context, run *domain.SandboxRun, policy *PolicySnapshot) error {
	if !policy.KillOnBreach {
		return nil
	}

	// Create a child context for the watcher
	watchCtx, cancel := context.WithCancel(ctx)

	// Store the cancel function
	p.mu.Lock()
	p.active[run.ID] = cancel
	p.mu.Unlock()

	// Start the watcher goroutine
	go p.watch(watchCtx, run, policy)

	return nil
}

// Disarm stops the watcher for the given sandbox ID.
// Safe to call multiple times.
func (p *PollFury) Disarm(ctx context.Context, runID domain.SandboxID) error {
	p.mu.Lock()
	cancel, exists := p.active[runID]
	if exists {
		delete(p.active, runID)
	}
	p.mu.Unlock()

	if exists {
		cancel()
	}

	return nil
}

// watch is the polling loop that monitors a sandbox run.
func (p *PollFury) watch(ctx context.Context, run *domain.SandboxRun, policy *PolicySnapshot) {
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkAndEnforce(ctx, run, policy)
		}
	}
}

// checkAndEnforce inspects the sandbox and enforces policy limits.
func (p *PollFury) checkAndEnforce(ctx context.Context, run *domain.SandboxRun, policy *PolicySnapshot) {
	// Inspect the current state
	currentRun, err := p.Runtime.Inspect(ctx, run.ID)
	if err != nil {
		p.Logger.Error(ctx, "Failed to inspect sandbox", map[string]any{
			"sandbox_id": run.ID,
			"error":      err.Error(),
		})
		return
	}

	// Check if the run has finished
	if isFinished(currentRun.Status) {
		p.stopWatching(run.ID)
		return
	}

	// Check runtime limit
	if policy.MaxRuntime > 0 {
		elapsed := time.Since(currentRun.StartedAt)
		if elapsed > policy.MaxRuntime {
			p.killForViolation(ctx, run.ID, "runtime_exceeded", map[string]any{
				"sandbox_id":  run.ID,
				"elapsed":     elapsed.String(),
				"max_runtime": policy.MaxRuntime.String(),
			})
			return
		}
	}

	// TODO: Check memory limit when memory metrics are available
	// For now, SandboxRun doesn't include current memory usage,
	// so we can't enforce memory limits yet.
	// Future implementation would check:
	// if policy.MaxMemory > 0 && currentRun.MemoryUsage > policy.MaxMemory {
	//     p.killForViolation(ctx, run.ID, "memory_exceeded", ...)
	// }
}

// killForViolation kills a sandbox for policy violation.
func (p *PollFury) killForViolation(ctx context.Context, runID domain.SandboxID, reason string, fields map[string]any) {
	// Log the violation
	fields["reason"] = reason
	p.Logger.Error(ctx, "Killing sandbox for policy violation", fields)

	// Emit metrics
	p.Metrics.IncCounter("erinyes_kill_total", 1, hermes.Label{
		Key:   "reason",
		Value: reason,
	})

	// Kill the sandbox
	if err := p.Runtime.Kill(ctx, runID); err != nil {
		p.Logger.Error(ctx, "Failed to kill sandbox", map[string]any{
			"sandbox_id": runID,
			"error":      err.Error(),
		})
	}

	// Stop watching
	p.stopWatching(runID)
}

// stopWatching stops the watcher for a given sandbox ID.
func (p *PollFury) stopWatching(runID domain.SandboxID) {
	p.mu.Lock()
	cancel, exists := p.active[runID]
	if exists {
		delete(p.active, runID)
	}
	p.mu.Unlock()

	if exists {
		cancel()
	}
}

// isFinished checks if a run status represents a finished state.
func isFinished(status domain.RunStatus) bool {
	return status == domain.RunStatusSucceeded ||
		status == domain.RunStatusFailed ||
		status == domain.RunStatusCanceled
}
