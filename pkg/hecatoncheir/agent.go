package hecatoncheir

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"strings"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/tartarus-sandbox/tartarus/pkg/hypnos"
	"github.com/vishvananda/netlink"

	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/cocytus"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/lethe"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
	"github.com/tartarus-sandbox/tartarus/pkg/styx"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
	"github.com/tartarus-sandbox/tartarus/pkg/thanatos"
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
	Hypnos     *hypnos.Manager
	Thanatos   *thanatos.Handler
	Queue      acheron.Queue
	Registry   hades.Registry
	DeadLetter cocytus.Sink
	Control    ControlListener
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

	// Start Control Loop
	if a.Control != nil {
		controlCh, err := a.Control.Listen(ctx)
		if err != nil {
			a.Logger.Error(ctx, "Failed to start control listener", map[string]any{"error": err})
		} else {
			go a.controlLoop(ctx, controlCh)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Dequeue
			req, receipt, err := a.Queue.Dequeue(ctx)
			if err != nil {
				a.Logger.Error(ctx, "Failed to dequeue", map[string]any{"error": err})
				time.Sleep(1 * time.Second)
				continue
			}

			a.Logger.Info(ctx, "Received request", map[string]any{"id": req.ID})
			a.Metrics.IncCounter("agent_jobs_dequeued_total", 1)

			// 1. Get Snapshot (Nyx)
			snap, err := a.Nyx.GetSnapshot(ctx, req.Template)
			if err != nil {
				a.Logger.Error(ctx, "Failed to get snapshot", map[string]any{"error": err})
				// If we can't get snapshot, it's likely a permanent error or configuration issue.
				// We should Nack (maybe with delay) or just Ack and fail.
				// For now, let's Nack to retry.
				a.Queue.Nack(ctx, receipt, "failed to get snapshot")
				a.Metrics.IncCounter("agent_jobs_failed_total", 1, hermes.Label{Key: "reason", Value: "snapshot_fetch_failed"})
				continue
			}

			// 2. Create Overlay (Lethe)
			overlay, err := a.Lethe.Create(ctx, snap)
			if err != nil {
				a.Logger.Error(ctx, "Failed to create overlay", map[string]any{"error": err})
				a.Queue.Nack(ctx, receipt, "failed to create overlay")
				a.Metrics.IncCounter("agent_jobs_failed_total", 1, hermes.Label{Key: "reason", Value: "overlay_creation_failed"})
				continue
			}

			// 3. Attach Network (Styx)
			contract := &styx.Contract{
				ID: req.NetworkRef.ID,
			}
			tapName, ip, gateway, cidr, err := a.Styx.Attach(ctx, req.ID, contract)
			if err != nil {
				a.Logger.Error(ctx, "Failed to attach network", map[string]any{"error": err})
				a.Lethe.Destroy(ctx, overlay)
				a.Queue.Nack(ctx, receipt, "failed to attach network")
				a.Metrics.IncCounter("agent_jobs_failed_total", 1, hermes.Label{Key: "reason", Value: "network_attach_failed"})
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
				IP:        ip,
				Gateway:   gateway,
				CIDR:      cidr,
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

				// Nack or Ack? If launch failed, it might be transient.
				a.Queue.Nack(ctx, receipt, "failed to launch")
				a.Metrics.IncCounter("agent_jobs_failed_total", 1, hermes.Label{Key: "reason", Value: "launch_failed"})
				continue
			}

			a.Logger.Info(ctx, "Sandbox launched", map[string]any{"run_id": run.ID})
			a.Metrics.IncCounter("agent_jobs_launched_total", 1)
			if !req.CreatedAt.IsZero() {
				latency := time.Since(req.CreatedAt).Seconds()
				a.Metrics.ObserveHistogram("agent_launch_latency_seconds", latency)
			}

			// Update Run Status to Running
			if err := a.Registry.UpdateRun(ctx, *run); err != nil {
				a.Logger.Error(ctx, "Failed to update run status", map[string]any{"run_id": run.ID, "error": err})
			}

			// Arm Watchdog (Erinyes)
			policy := &erinyes.PolicySnapshot{
				MaxRuntime:   req.Resources.TTL,
				KillOnBreach: true,
			}
			if err := a.Furies.Arm(ctx, run, policy); err != nil {
				a.Logger.Error(ctx, "Failed to arm watchdog", map[string]any{"run_id": run.ID, "error": err})
			}

			// 5. Wait & Cleanup
			go func(runID domain.SandboxID, reqID domain.SandboxID, ov *lethe.Overlay, receipt string) {
				// Wait for completion
				if err := a.Runtime.Wait(context.Background(), runID); err != nil {
					a.Logger.Error(context.Background(), "Wait failed", map[string]any{"run_id": runID, "error": err})
				}

				a.Logger.Info(context.Background(), "Sandbox exited", map[string]any{"run_id": runID})

				// Disarm Watchdog
				if err := a.Furies.Disarm(context.Background(), runID); err != nil {
					a.Logger.Error(context.Background(), "Failed to disarm watchdog", map[string]any{"run_id": runID, "error": err})
				}

				// Inspect to get final status and exit code
				finalRun, err := a.Runtime.Inspect(context.Background(), runID)
				if err == nil {
					// Update Run Status to Succeeded/Failed
					if err := a.Registry.UpdateRun(context.Background(), *finalRun); err != nil {
						a.Logger.Error(context.Background(), "Failed to update final run status", map[string]any{"run_id": runID, "error": err})
					}
				} else {
					a.Logger.Error(context.Background(), "Failed to inspect final run", map[string]any{"run_id": runID, "error": err})
				}

				// Cleanup Network
				if err := a.Styx.Detach(context.Background(), reqID); err != nil {
					a.Logger.Error(context.Background(), "Failed to detach network", map[string]any{"req_id": reqID, "error": err})
				}

				// Cleanup Overlay
				if err := a.Lethe.Destroy(context.Background(), ov); err != nil {
					a.Logger.Error(context.Background(), "Failed to destroy overlay", map[string]any{"overlay_id": ov.ID, "error": err})
				}

				// Ack the job
				if err := a.Queue.Ack(context.Background(), receipt); err != nil {
					a.Logger.Error(context.Background(), "Failed to ack job", map[string]any{"req_id": reqID, "error": err})
				}
				// We can't easily access 'a.Metrics' here if it's not thread-safe or if we are in a closure?
				// 'a' is available.
				// But we are in a goroutine.
				// Assuming Metrics is thread-safe (SlogAdapter is).
				// We should emit success/failure based on exit code?
				// But we don't have exit code easily here unless we check finalRun.
				// Let's just emit "finished".
				// Actually, we can check if finalRun.ExitCode == 0
				// But finalRun might be nil if Inspect failed.
				// Let's just emit "job_finished".
			}(run.ID, req.ID, overlay, receipt)
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

func (a *Agent) controlLoop(ctx context.Context, ch <-chan ControlMessage) {
	a.Logger.Info(ctx, "Control loop started", nil)
	for msg := range ch {
		a.Logger.Info(ctx, "Received control message", map[string]any{"type": msg.Type, "sandbox_id": msg.SandboxID})

		switch msg.Type {
		case ControlMessageKill:
			if err := a.Runtime.Kill(ctx, msg.SandboxID); err != nil {
				a.Logger.Error(ctx, "Failed to kill sandbox", map[string]any{"sandbox_id": msg.SandboxID, "error": err})
			} else {
				a.Logger.Info(ctx, "Killed sandbox", map[string]any{"sandbox_id": msg.SandboxID})
			}
		case ControlMessageLogs:
			go a.streamLogs(ctx, msg.SandboxID)
		case ControlMessageHibernate:
			a.Logger.Info(ctx, "Hibernating sandbox", map[string]any{"sandbox_id": msg.SandboxID})
			if _, err := a.Hypnos.Sleep(ctx, msg.SandboxID, nil); err != nil {
				a.Logger.Error(ctx, "Failed to hibernate sandbox", map[string]any{"sandbox_id": msg.SandboxID, "error": err})
			}
		case ControlMessageTerminate:
			a.Logger.Info(ctx, "Terminating sandbox", map[string]any{"sandbox_id": msg.SandboxID})
			if _, err := a.Thanatos.Terminate(ctx, msg.SandboxID, thanatos.Options{GracePeriod: 5 * time.Second}); err != nil {
				a.Logger.Error(ctx, "Failed to terminate sandbox", map[string]any{"sandbox_id": msg.SandboxID, "error": err})
			}
		}
	}
}

func (a *Agent) streamLogs(ctx context.Context, id domain.SandboxID) {
	// Create a pipe to read logs from runtime and write to Redis
	r, w := io.Pipe()

	// Goroutine to publish to Redis
	go func() {
		defer r.Close()
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				if pErr := a.Control.PublishLogs(ctx, id, buf[:n]); pErr != nil {
					a.Logger.Error(ctx, "Failed to publish logs", map[string]any{"sandbox_id": id, "error": pErr})
					// If we can't publish, maybe we should stop?
					// For now, just log error and continue
				}
			}
			if err != nil {
				if err != io.EOF {
					a.Logger.Error(ctx, "Error reading logs", map[string]any{"sandbox_id": id, "error": err})
				}
				return
			}
		}
	}()

	// Stream from Runtime to Pipe Writer
	if err := a.Runtime.StreamLogs(ctx, id, w); err != nil {
		a.Logger.Error(ctx, "Failed to stream logs from runtime", map[string]any{"sandbox_id": id, "error": err})
	}
	w.Close()
}
