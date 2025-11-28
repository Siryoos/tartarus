package hecatoncheir

import (
	"context"
	"encoding/json"
	"time"

	"strings"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/vishvananda/netlink"

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

	if err := a.Reconcile(ctx); err != nil {
		a.Logger.Error(ctx, "Reconciliation failed", map[string]any{"error": err})
		// We continue even if reconciliation fails, but logging is critical
	}

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

				// Report to Cocytus
				go func() {
					payload, _ := json.Marshal(req)
					rec := &cocytus.Record{
						RequestID: req.ID,
						Reason:    err.Error(),
						Payload:   payload,
						CreatedAt: time.Now(),
					}
					// Use a detached context with timeout to avoid blocking
					rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					if wErr := a.DeadLetter.Write(rctx, rec); wErr != nil {
						a.Logger.Error(context.Background(), "Failed to write to dead letter sink", map[string]any{"error": wErr})
					}
				}()

				// Cleanup
				a.Styx.Detach(ctx, req.ID)
				a.Lethe.Destroy(ctx, overlay)
				continue
			}

			a.Logger.Info(ctx, "Sandbox launched", map[string]any{"run_id": run.ID})

			// 5. Wait & Cleanup
			go func(runID domain.SandboxID, reqID domain.SandboxID, ov *lethe.Overlay) {
				// Wait for completion
				if err := a.Runtime.Wait(context.Background(), runID); err != nil {
					a.Logger.Error(context.Background(), "Wait failed", map[string]any{"run_id": runID, "error": err})
				}

				a.Logger.Info(context.Background(), "Sandbox exited", map[string]any{"run_id": runID})

				// Cleanup Network
				if err := a.Styx.Detach(context.Background(), reqID); err != nil {
					a.Logger.Error(context.Background(), "Failed to detach network", map[string]any{"req_id": reqID, "error": err})
				}

				// Cleanup Overlay
				if err := a.Lethe.Destroy(context.Background(), ov); err != nil {
					a.Logger.Error(context.Background(), "Failed to destroy overlay", map[string]any{"overlay_id": ov.ID, "error": err})
				}
			}(run.ID, req.ID, overlay)
		}
	}
}

// Reconcile cleans up zombie processes and network interfaces from previous runs.
func (a *Agent) Reconcile(ctx context.Context) error {
	a.Logger.Info(ctx, "Starting reconciliation", nil)

	// 1. Network Cleanup
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}

	for _, link := range links {
		name := link.Attrs().Name
		if strings.HasPrefix(name, "tap-tartarus-") {
			a.Logger.Info(ctx, "Cleaning up zombie interface", map[string]any{"interface": name})
			if err := netlink.LinkDel(link); err != nil {
				a.Logger.Error(ctx, "Failed to delete zombie interface", map[string]any{"interface": name, "error": err})
			}
		}
	}

	// 2. Process Cleanup
	procs, err := process.Processes()
	if err != nil {
		return err
	}

	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}

		if name == "firecracker" {
			cmdline, err := p.Cmdline()
			if err != nil {
				continue
			}

			if strings.Contains(cmdline, "tartarus") {
				pid := p.Pid
				a.Logger.Info(ctx, "Killing zombie firecracker process", map[string]any{"pid": pid})
				if err := p.Kill(); err != nil {
					a.Logger.Error(ctx, "Failed to kill zombie process", map[string]any{"pid": pid, "error": err})
				}
			}
		}
	}

	a.Logger.Info(ctx, "Reconciliation complete", nil)
	return nil
}
