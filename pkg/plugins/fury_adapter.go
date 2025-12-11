package plugins

import (
	"context"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
)

// FuryPluginAdapter wraps a FuryPlugin to implement erinyes.Fury.
type FuryPluginAdapter struct {
	plugin FuryPlugin
}

// NewFuryPluginAdapter creates a new adapter for a fury plugin.
func NewFuryPluginAdapter(plugin FuryPlugin) *FuryPluginAdapter {
	return &FuryPluginAdapter{plugin: plugin}
}

// Arm implements erinyes.Fury.
func (a *FuryPluginAdapter) Arm(ctx context.Context, run *domain.SandboxRun, policy *erinyes.PolicySnapshot) error {
	// Convert erinyes.PolicySnapshot to plugins.PolicySnapshot
	pluginPolicy := &PolicySnapshot{
		MaxRuntimeSeconds:      int64(policy.MaxRuntime / time.Second),
		MaxCPUMillis:           int64(policy.MaxCPU),
		MaxMemoryMB:            int64(policy.MaxMemory),
		MaxNetworkEgressBytes:  policy.MaxNetworkEgressBytes,
		MaxNetworkIngressBytes: policy.MaxNetworkIngressBytes,
		MaxBannedIPAttempts:    policy.MaxBannedIPAttempts,
		KillOnBreach:           policy.KillOnBreach,
	}
	return a.plugin.Arm(ctx, run, pluginPolicy)
}

// Disarm implements erinyes.Fury.
func (a *FuryPluginAdapter) Disarm(ctx context.Context, runID domain.SandboxID) error {
	return a.plugin.Disarm(ctx, runID)
}

// Name returns the plugin name.
func (a *FuryPluginAdapter) Name() string {
	return a.plugin.Name()
}

// CompositeFury combines multiple Fury implementations into one.
type CompositeFury struct {
	furies []erinyes.Fury
	mu     sync.RWMutex
}

// NewCompositeFury creates a composite fury from multiple furies.
func NewCompositeFury(furies ...erinyes.Fury) *CompositeFury {
	return &CompositeFury{
		furies: furies,
	}
}

// AddFury adds a fury to the composite.
func (c *CompositeFury) AddFury(fury erinyes.Fury) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.furies = append(c.furies, fury)
}

// Arm arms all furies.
func (c *CompositeFury) Arm(ctx context.Context, run *domain.SandboxRun, policy *erinyes.PolicySnapshot) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, fury := range c.furies {
		if err := fury.Arm(ctx, run, policy); err != nil {
			return err
		}
	}
	return nil
}

// Disarm disarms all furies.
func (c *CompositeFury) Disarm(ctx context.Context, runID domain.SandboxID) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var lastErr error
	for _, fury := range c.furies {
		if err := fury.Disarm(ctx, runID); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// WrapFuryPlugins wraps multiple fury plugins as erinyes.Fury.
func WrapFuryPlugins(plugins []FuryPlugin) []erinyes.Fury {
	var furies []erinyes.Fury
	for _, p := range plugins {
		furies = append(furies, NewFuryPluginAdapter(p))
	}
	return furies
}
