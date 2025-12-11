//go:build ignore
// +build ignore

// This is an example plugin source file.
// Build with: go build -buildmode=plugin -o cost-aware-fury.so main.go
//
// Note: Go plugins only work on Linux with matching Go versions.

package main

import (
	"context"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/plugins"
)

// CostAwareFury monitors sandbox costs and terminates when limits are exceeded.
type CostAwareFury struct {
	maxCostPerHour  float64
	cpuCostPerCore  float64
	memoryCostPerGB float64
	checkInterval   time.Duration

	mu       sync.Mutex
	watchers map[domain.SandboxID]context.CancelFunc
	costs    map[domain.SandboxID]float64
}

func (c *CostAwareFury) Name() string {
	return "cost-aware-fury"
}

func (c *CostAwareFury) Version() string {
	return "1.0.0"
}

func (c *CostAwareFury) Type() plugins.PluginType {
	return plugins.PluginTypeFury
}

func (c *CostAwareFury) Init(config map[string]any) error {
	c.watchers = make(map[domain.SandboxID]context.CancelFunc)
	c.costs = make(map[domain.SandboxID]float64)

	// Parse config
	if v, ok := config["maxCostPerHour"].(float64); ok {
		c.maxCostPerHour = v
	} else {
		c.maxCostPerHour = 10.0
	}

	if v, ok := config["cpuCostPerCore"].(float64); ok {
		c.cpuCostPerCore = v
	} else {
		c.cpuCostPerCore = 0.05
	}

	if v, ok := config["memoryCostPerGB"].(float64); ok {
		c.memoryCostPerGB = v
	} else {
		c.memoryCostPerGB = 0.01
	}

	if v, ok := config["checkInterval"].(string); ok {
		d, err := time.ParseDuration(v)
		if err == nil {
			c.checkInterval = d
		}
	}
	if c.checkInterval == 0 {
		c.checkInterval = 30 * time.Second
	}

	return nil
}

func (c *CostAwareFury) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, cancel := range c.watchers {
		cancel()
	}
	c.watchers = make(map[domain.SandboxID]context.CancelFunc)
	return nil
}

func (c *CostAwareFury) Arm(ctx context.Context, run *domain.SandboxRun, policy *plugins.PolicySnapshot) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create cancellable context for this watcher
	watchCtx, cancel := context.WithCancel(ctx)
	c.watchers[run.ID] = cancel
	c.costs[run.ID] = 0

	// Start cost monitoring goroutine
	go c.monitor(watchCtx, run, policy)

	return nil
}

func (c *CostAwareFury) Disarm(ctx context.Context, runID domain.SandboxID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cancel, exists := c.watchers[runID]; exists {
		cancel()
		delete(c.watchers, runID)
		delete(c.costs, runID)
	}

	return nil
}

func (c *CostAwareFury) monitor(ctx context.Context, run *domain.SandboxRun, policy *plugins.PolicySnapshot) {
	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Calculate cost based on resources
			elapsed := time.Since(startTime)
			hours := elapsed.Hours()

			// Simplified cost calculation
			cpuCores := float64(policy.MaxCPUMillis) / 1000.0
			memoryGB := float64(policy.MaxMemoryMB) / 1024.0

			cost := (cpuCores * c.cpuCostPerCore * hours) +
				(memoryGB * c.memoryCostPerGB * hours)

			c.mu.Lock()
			c.costs[run.ID] = cost
			exceeded := cost >= c.maxCostPerHour
			c.mu.Unlock()

			if exceeded {
				// In a real implementation, we would call the runtime to kill
				// the sandbox here. This example just demonstrates the structure.
				return
			}
		}
	}
}

// TartarusPlugin is the exported symbol that the plugin loader looks for.
var TartarusPlugin plugins.FuryPlugin = &CostAwareFury{}
