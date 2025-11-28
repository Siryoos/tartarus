package hecatoncheir

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/lethe"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
	"github.com/tartarus-sandbox/tartarus/pkg/styx"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
)

// Agent is the hundred-handed guardian on a node.

type Agent struct {
	NodeID     domain.NodeID
	Runtime    tartarus.SandboxRuntime
	Nyx        nyx.Manager
	Lethe      lethe.Pool
	Styx       styx.Gateway
	Judges     *judges.Chain
	Furies     erinyes.Fury
	Queue      acheron.Queue
	DeadLetter cocytus.Sink
	Metrics    hermes.Metrics
	Logger     hermes.Logger
}

// Run starts the main loop: consume from Acheron, execute, enforce, report.

func (a *Agent) Run(ctx context.Context) error {
	a.Logger.Info(ctx, "Agent starting", nil)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Dequeue
			req, err := a.Queue.Dequeue(ctx)
			if err != nil {
				a.Logger.Error(ctx, "Failed to dequeue", map[string]any{"error": err})
				time.Sleep(1 * time.Second)
				continue
			}

			a.Logger.Info(ctx, "Received request", map[string]any{"id": req.ID})

			// 1. Get Snapshot (Nyx)
			snap, err := a.Nyx.GetSnapshot(ctx, req.Template)
			if err != nil {
				a.Logger.Error(ctx, "Failed to get snapshot", map[string]any{"error": err})
				continue
			}

			// 2. Create Overlay (Lethe)
			overlay, err := a.Lethe.Create(ctx, snap)
			if err != nil {
				a.Logger.Error(ctx, "Failed to create overlay", map[string]any{"error": err})
				continue
			}

			// 3. Attach Network (Styx)
			contract := &styx.Contract{
				ID: req.NetworkRef.ID,
			}
			tapName, _, err := a.Styx.Attach(ctx, req.ID, contract)
			if err != nil {
				a.Logger.Error(ctx, "Failed to attach network", map[string]any{"error": err})
				a.Lethe.Destroy(ctx, overlay)
				continue
			}

			// 4. Launch (Runtime)
			vmCfg := tartarus.VMConfig{
				Snapshot: domain.SnapshotRef{
					ID:       snap.ID,
					Template: snap.Template,
					Path:     snap.Path,
				},
				OverlayFS: overlay.MountPath,
				TapDevice: tapName,
				CPUs:      int(req.Resources.CPU),
				MemoryMB:  int(req.Resources.Mem),
			}

			run, err := a.Runtime.Launch(ctx, req, vmCfg)
			if err != nil {
				a.Logger.Error(ctx, "Failed to launch", map[string]any{"error": err})
				// Cleanup
				a.Styx.Detach(ctx, req.ID)
				a.Lethe.Destroy(ctx, overlay)
				continue
			}

			a.Logger.Info(ctx, "Sandbox launched", map[string]any{"run_id": run.ID})
		}
	}
}
