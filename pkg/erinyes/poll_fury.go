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
	Runtime      tartarus.SandboxRuntime
	Logger       hermes.Logger
	Metrics      hermes.Metrics
	NetworkStats NetworkStatsProvider
	Interval     time.Duration

	mu     sync.Mutex
	active map[domain.SandboxID]context.CancelFunc
}

// NewPollFury creates a new PollFury instance.
func NewPollFury(runtime tartarus.SandboxRuntime, logger hermes.Logger, metrics hermes.Metrics, networkStats NetworkStatsProvider, interval time.Duration) *PollFury {
	return &PollFury{
		Runtime:      runtime,
		Logger:       logger,
		Metrics:      metrics,
		NetworkStats: networkStats,
		Interval:     interval,
		active:       make(map[domain.SandboxID]context.CancelFunc),
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

	// Check memory limit
	if policy.MaxMemory > 0 && currentRun.MemoryUsage > policy.MaxMemory {
		p.killForViolation(ctx, run.ID, "memory_exceeded", map[string]any{
			"sandbox_id":   run.ID,
			"memory_usage": currentRun.MemoryUsage,
			"max_memory":   policy.MaxMemory,
		})
		return
	}

	// Check network limits
	// We need the TAP device name from the runtime config
	cfg, _, err := p.Runtime.GetConfig(ctx, run.ID)
	if err != nil {
		p.Logger.Error(ctx, "Failed to get config for network enforcement", map[string]any{
			"sandbox_id": run.ID,
			"error":      err.Error(),
		})
		return
	}

	if cfg.TapDevice != "" {
		// Get interface stats
		rx, tx, err := p.NetworkStats.GetInterfaceStats(ctx, cfg.TapDevice)
		if err != nil {
			p.Logger.Error(ctx, "Failed to get network stats", map[string]any{
				"sandbox_id": run.ID,
				"tap_device": cfg.TapDevice,
				"error":      err.Error(),
			})
		} else {
			// Host RX = VM Egress
			if policy.MaxNetworkEgressBytes > 0 && rx > policy.MaxNetworkEgressBytes {
				p.killForViolation(ctx, run.ID, "network_egress_exceeded", map[string]any{
					"sandbox_id": run.ID,
					"egress":     rx,
					"max_egress": policy.MaxNetworkEgressBytes,
				})
				return
			}
			// Host TX = VM Ingress
			if policy.MaxNetworkIngressBytes > 0 && tx > policy.MaxNetworkIngressBytes {
				p.killForViolation(ctx, run.ID, "network_ingress_exceeded", map[string]any{
					"sandbox_id":  run.ID,
					"ingress":     tx,
					"max_ingress": policy.MaxNetworkIngressBytes,
				})
				return
			}
		}

		// Check banned IP attempts
		if policy.MaxBannedIPAttempts > 0 {
			drops, err := p.NetworkStats.GetDropCount(ctx, cfg.TapDevice)
			if err != nil {
				p.Logger.Error(ctx, "Failed to get drop count", map[string]any{
					"sandbox_id": run.ID,
					"tap_device": cfg.TapDevice,
					"error":      err.Error(),
				})
			} else if drops > policy.MaxBannedIPAttempts {
				p.killForViolation(ctx, run.ID, "banned_ip_attempts_exceeded", map[string]any{
					"sandbox_id":   run.ID,
					"drops":        drops,
					"max_attempts": policy.MaxBannedIPAttempts,
				})
				return
			}
		}
	}
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
