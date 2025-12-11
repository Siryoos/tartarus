# Writing Custom Furies

Furies enforce runtime policies on running sandboxes, monitoring resource usage and terminating violators.

## Fury Interface

```go
type Fury interface {
    // Arm starts enforcement for a sandbox
    Arm(ctx context.Context, run *domain.SandboxRun, policy *PolicySnapshot) error
    
    // Disarm stops enforcement when sandbox completes
    Disarm(ctx context.Context, runID domain.SandboxID) error
}
```

## Policy Snapshot

```go
type PolicySnapshot struct {
    MaxRuntime             time.Duration
    MaxCPU                 domain.MilliCPU
    MaxMemory              domain.Megabytes
    MaxNetworkEgressBytes  int64
    MaxNetworkIngressBytes int64
    MaxBannedIPAttempts    int
    KillOnBreach           bool
}
```

## Creating a Plugin Fury

### 1. Create Manifest

```yaml
apiVersion: v1
kind: TartarusPlugin
metadata:
  name: cost-monitor
  version: 1.0.0
  description: Monitors sandbox costs
spec:
  type: fury
  entryPoint: cost-monitor.so
  config:
    maxCostPerHour: 10.0
```

### 2. Implement the Plugin

```go
package main

import (
    "context"
    "sync"
    "time"
    
    "github.com/tartarus-sandbox/tartarus/pkg/domain"
    "github.com/tartarus-sandbox/tartarus/pkg/plugins"
)

type CostMonitor struct {
    maxCost  float64
    watchers map[domain.SandboxID]context.CancelFunc
    mu       sync.Mutex
}

func (c *CostMonitor) Name() string              { return "cost-monitor" }
func (c *CostMonitor) Version() string           { return "1.0.0" }
func (c *CostMonitor) Type() plugins.PluginType  { return plugins.PluginTypeFury }

func (c *CostMonitor) Init(config map[string]any) error {
    c.watchers = make(map[domain.SandboxID]context.CancelFunc)
    if v, ok := config["maxCostPerHour"].(float64); ok {
        c.maxCost = v
    }
    return nil
}

func (c *CostMonitor) Close() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    for _, cancel := range c.watchers {
        cancel()
    }
    return nil
}

func (c *CostMonitor) Arm(ctx context.Context, run *domain.SandboxRun, policy *plugins.PolicySnapshot) error {
    watchCtx, cancel := context.WithCancel(ctx)
    
    c.mu.Lock()
    c.watchers[run.ID] = cancel
    c.mu.Unlock()
    
    go c.monitor(watchCtx, run, policy)
    return nil
}

func (c *CostMonitor) Disarm(ctx context.Context, runID domain.SandboxID) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if cancel, ok := c.watchers[runID]; ok {
        cancel()
        delete(c.watchers, runID)
    }
    return nil
}

func (c *CostMonitor) monitor(ctx context.Context, run *domain.SandboxRun, policy *plugins.PolicySnapshot) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // Calculate and check cost
        }
    }
}

var TartarusPlugin plugins.FuryPlugin = &CostMonitor{}
```

### 3. Build and Install

```bash
go build -buildmode=plugin -o cost-monitor.so main.go
tartarus plugin install ./cost-monitor
```

## Built-in Furies

| Fury | Description |
|------|-------------|
| `PollFury` | Polls sandbox metrics and enforces limits |
