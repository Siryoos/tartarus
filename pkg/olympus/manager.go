package olympus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
	"github.com/tartarus-sandbox/tartarus/pkg/nyx"
	"github.com/tartarus-sandbox/tartarus/pkg/phlegethon"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

var ErrPolicyRejected = errors.New("request rejected by policy enforcement")
var ErrSandboxNotFound = errors.New("sandbox not found")

// Manager is Olympus: front-door for users, back-door to Hades and Acheron.

type Manager struct {
	Queue      acheron.Queue
	Hades      hades.Registry
	Policies   themis.Repository
	Templates  TemplateManager
	Nyx        nyx.Manager
	Judges     *judges.Chain
	Scheduler  moirai.Scheduler
	Phlegethon *phlegethon.HeatClassifier
	Control    ControlPlane
	Metrics    hermes.Metrics
	Logger     hermes.Logger
}

// Submit enqueues a new sandbox request after validation and policy checks.

func (m *Manager) Submit(ctx context.Context, req *domain.SandboxRequest) error {
	// 1) Assign ID if missing
	if req.ID == "" {
		req.ID = domain.SandboxID(uuid.New().String())
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}

	start := time.Now()
	defer func() {
		m.Metrics.ObserveHistogram("sandbox_submission_duration_seconds", time.Since(start).Seconds())
	}()

	m.Metrics.IncCounter("sandbox_submissions_total", 1)

	// 2) Validate Template
	_, err := m.Templates.GetTemplate(ctx, req.Template)
	if err != nil {
		m.Logger.Error(ctx, "Template not found", map[string]any{
			"template": req.Template,
			"error":    err,
		})
		m.Metrics.IncCounter("sandbox_submission_failures_total", 1, hermes.Label{Key: "reason", Value: "invalid_template"})
		return fmt.Errorf("invalid template: %w", err)
	}

	// 3) Load policy from Themis
	policy, err := m.Policies.GetPolicy(ctx, req.Template)
	if err != nil {
		m.Logger.Error(ctx, "Failed to load policy", map[string]any{
			"template": req.Template,
			"error":    err,
		})
		m.Metrics.IncCounter("sandbox_submission_failures_total", 1, hermes.Label{Key: "reason", Value: "policy_load_failed"})
		return err
	}

	m.Logger.Info(ctx, "Loaded policy for request", map[string]any{
		"sandbox_id": req.ID,
		"template":   req.Template,
		"policy_id":  policy.ID,
	})

	// 4) Run PreJudges
	verdict, err := m.Judges.RunPre(ctx, req)
	if err != nil {
		m.Logger.Error(ctx, "Judge evaluation failed", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		m.Metrics.IncCounter("sandbox_submission_failures_total", 1, hermes.Label{Key: "reason", Value: "judge_error"})
		return err
	}

	// 5) Verdict Handling
	switch verdict {
	case judges.VerdictReject:
		m.Logger.Info(ctx, "Request rejected by policy enforcement", map[string]any{
			"sandbox_id": req.ID,
			"verdict":    verdict,
		})
		m.Metrics.IncCounter("sandbox_submission_failures_total", 1, hermes.Label{Key: "reason", Value: "rejected"})
		return ErrPolicyRejected
	case judges.VerdictQuarantine:
		m.Logger.Info(ctx, "Request quarantined by policy enforcement", map[string]any{
			"sandbox_id": req.ID,
			"verdict":    verdict,
		})
		if req.Metadata == nil {
			req.Metadata = make(map[string]string)
		}
		req.Metadata["quarantine"] = "true"
	case judges.VerdictAccept:
		m.Logger.Info(ctx, "Request passed all judges", map[string]any{
			"sandbox_id": req.ID,
		})
	default:
		return fmt.Errorf("unknown verdict: %v", verdict)
	}

	// 6) Persistence
	initialRun := domain.SandboxRun{
		ID:        req.ID,
		RequestID: req.ID,
		Template:  req.Template,
		Status:    domain.RunStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := m.Hades.UpdateRun(ctx, initialRun); err != nil {
		m.Logger.Error(ctx, "Failed to persist initial run state", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		m.Metrics.IncCounter("sandbox_submission_failures_total", 1, hermes.Label{Key: "reason", Value: "persistence_failed"})
		return fmt.Errorf("failed to persist run state: %w", err)
	}

	// 7) Heat Classification
	if m.Phlegethon != nil {
		// Map domain.SandboxRequest to phlegethon.SandboxRequest
		phlegReq := &phlegethon.SandboxRequest{
			TemplateID:  string(req.Template),
			MaxDuration: req.Resources.TTL,
			CPUCores:    int(req.Resources.CPU / 1000), // Convert milliCPU to cores
			MemoryMB:    int(req.Resources.Mem),
		}

		// Check for explicit heat hint in metadata
		if req.Metadata != nil {
			if heatHint := req.Metadata["heat_hint"]; heatHint != "" {
				phlegReq.HeatHint = phlegethon.HeatLevel(heatHint)
			}
		}

		heatLevel, source := m.Phlegethon.Classify(phlegReq)
		req.HeatLevel = string(heatLevel)

		m.Logger.Info(ctx, "Classified workload heat", map[string]any{
			"sandbox_id": req.ID,
			"heat_level": heatLevel,
			"source":     source,
			"cpu_cores":  phlegReq.CPUCores,
			"memory_mb":  phlegReq.MemoryMB,
			"ttl":        phlegReq.MaxDuration,
		})

		m.Metrics.IncCounter("phlegethon_classification_total", 1,
			hermes.Label{Key: "heat_level", Value: string(heatLevel)},
			hermes.Label{Key: "source", Value: source},
		)
	}

	// 8) Scheduling
	nodes, err := m.Hades.ListNodes(ctx)
	if err != nil {
		m.Logger.Error(ctx, "Failed to list nodes for scheduling", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		// Mark as failed
		initialRun.Status = domain.RunStatusFailed
		initialRun.Error = fmt.Sprintf("failed to list nodes: %v", err)
		initialRun.UpdatedAt = time.Now()
		_ = m.Hades.UpdateRun(ctx, initialRun)
		m.Metrics.IncCounter("sandbox_submission_failures_total", 1, hermes.Label{Key: "reason", Value: "node_listing_failed"})
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	nodeID, err := m.Scheduler.ChooseNode(ctx, req, nodes)
	if err != nil {
		m.Logger.Error(ctx, "Failed to schedule sandbox", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		// Mark as failed
		initialRun.Status = domain.RunStatusFailed
		initialRun.Error = fmt.Sprintf("failed to schedule: %v", err)
		initialRun.UpdatedAt = time.Now()
		_ = m.Hades.UpdateRun(ctx, initialRun)
		m.Metrics.IncCounter("sandbox_submission_failures_total", 1, hermes.Label{Key: "reason", Value: "scheduling_failed"})
		return fmt.Errorf("failed to schedule sandbox: %w", err)
	}
	req.NodeID = nodeID

	// Update run with scheduled node
	initialRun.NodeID = nodeID
	initialRun.Status = domain.RunStatusScheduled
	initialRun.UpdatedAt = time.Now()
	if err := m.Hades.UpdateRun(ctx, initialRun); err != nil {
		m.Logger.Error(ctx, "Failed to update run state to SCHEDULED", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		// Continue anyway? If we fail to persist scheduled state, we might have an inconsistency.
		// But if we enqueue, the agent will eventually update it to RUNNING.
		// Let's log and continue.
	}

	m.Logger.Info(ctx, "Scheduled sandbox", map[string]any{
		"sandbox_id": req.ID,
		"node_id":    nodeID,
	})

	// 8) Enqueue into Acheron
	if err := m.Queue.Enqueue(ctx, req); err != nil {
		m.Logger.Error(ctx, "Failed to enqueue request", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		// Mark as failed
		initialRun.Status = domain.RunStatusFailed
		initialRun.Error = fmt.Sprintf("failed to enqueue: %v", err)
		initialRun.UpdatedAt = time.Now()
		_ = m.Hades.UpdateRun(ctx, initialRun)
		m.Metrics.IncCounter("sandbox_submission_failures_total", 1, hermes.Label{Key: "reason", Value: "enqueue_failed"})
		return err
	}

	m.Logger.Info(ctx, "Request successfully enqueued", map[string]any{
		"sandbox_id": req.ID,
	})
	return nil
}

// ListSandboxes returns all sandboxes across all nodes.
func (m *Manager) ListSandboxes(ctx context.Context) ([]domain.SandboxRun, error) {
	return m.Hades.ListRuns(ctx)
}

// KillSandbox sends a kill command to the node running the sandbox.
func (m *Manager) KillSandbox(ctx context.Context, id domain.SandboxID) error {
	// Find which node is running this sandbox
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		return ErrSandboxNotFound
	}

	if err := m.Control.Kill(ctx, run.NodeID, id); err != nil {
		m.Logger.Error(ctx, "Failed to send kill command", map[string]any{
			"sandbox_id": id,
			"node_id":    run.NodeID,
			"error":      err,
		})
		return err
	}

	m.Logger.Info(ctx, "Kill command sent", map[string]any{
		"sandbox_id": id,
		"node_id":    run.NodeID,
	})
	return nil
}

// HibernateSandbox sends a hibernate command to the node running the sandbox.
func (m *Manager) HibernateSandbox(ctx context.Context, id domain.SandboxID) error {
	// Find which node is running this sandbox
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		m.Metrics.IncCounter("sandbox_hibernate_failures_total", 1, hermes.Label{Key: "reason", Value: "not_found"})
		return ErrSandboxNotFound
	}

	if err := m.Control.Hibernate(ctx, run.NodeID, id); err != nil {
		m.Logger.Error(ctx, "Failed to send hibernate command", map[string]any{
			"sandbox_id": id,
			"node_id":    run.NodeID,
			"error":      err,
		})
		m.Metrics.IncCounter("sandbox_hibernate_failures_total", 1, hermes.Label{Key: "reason", Value: "control_error"})
		return err
	}

	m.Logger.Info(ctx, "Hibernate command sent", map[string]any{
		"sandbox_id": id,
		"node_id":    run.NodeID,
	})
	m.Metrics.IncCounter("sandbox_hibernate_requests_total", 1)
	return nil
}

// WakeSandbox sends a wake command to the node that hibernated the sandbox.
func (m *Manager) WakeSandbox(ctx context.Context, id domain.SandboxID) error {
	// Find which node has the hibernated sandbox
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		m.Metrics.IncCounter("sandbox_wake_failures_total", 1, hermes.Label{Key: "reason", Value: "not_found"})
		return ErrSandboxNotFound
	}

	if err := m.Control.Wake(ctx, run.NodeID, id); err != nil {
		m.Logger.Error(ctx, "Failed to send wake command", map[string]any{
			"sandbox_id": id,
			"node_id":    run.NodeID,
			"error":      err,
		})
		m.Metrics.IncCounter("sandbox_wake_failures_total", 1, hermes.Label{Key: "reason", Value: "control_error"})
		return err
	}

	m.Logger.Info(ctx, "Wake command sent", map[string]any{
		"sandbox_id": id,
		"node_id":    run.NodeID,
	})
	m.Metrics.IncCounter("sandbox_wake_requests_total", 1)
	return nil
}

// StreamLogs streams logs from the sandbox on the specified node.
func (m *Manager) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer, follow bool) error {
	// Find which node is running this sandbox
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		return ErrSandboxNotFound
	}

	if err := m.Control.StreamLogs(ctx, run.NodeID, id, w, follow); err != nil {
		m.Logger.Error(ctx, "Failed to stream logs", map[string]any{
			"sandbox_id": id,
			"node_id":    run.NodeID,
			"error":      err,
		})
		return err
	}

	return nil
}

// CreateSnapshot triggers a snapshot creation for the sandbox.
func (m *Manager) CreateSnapshot(ctx context.Context, id domain.SandboxID) error {
	// Find which node is running this sandbox
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		return ErrSandboxNotFound
	}

	if err := m.Control.Snapshot(ctx, run.NodeID, id); err != nil {
		m.Logger.Error(ctx, "Failed to send snapshot command", map[string]any{
			"sandbox_id": id,
			"node_id":    run.NodeID,
			"error":      err,
		})
		return err
	}

	m.Logger.Info(ctx, "Snapshot command sent", map[string]any{
		"sandbox_id": id,
		"node_id":    run.NodeID,
	})
	return nil
}

// ListSnapshots returns all snapshots for the template of the given sandbox.
func (m *Manager) ListSnapshots(ctx context.Context, id domain.SandboxID) ([]*nyx.Snapshot, error) {
	// Find the sandbox to get its template
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		return nil, ErrSandboxNotFound
	}

	return m.Nyx.ListSnapshots(ctx, run.Template)
}

// DeleteSnapshot deletes a snapshot.
func (m *Manager) DeleteSnapshot(ctx context.Context, id domain.SandboxID, snapID domain.SnapshotID) error {
	// Find the sandbox to get its template
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		return ErrSandboxNotFound
	}

	return m.Nyx.DeleteSnapshot(ctx, run.Template, snapID)
}

// Exec executes a command in the sandbox.
func (m *Manager) Exec(ctx context.Context, id domain.SandboxID, cmd []string) error {
	// Find which node is running this sandbox
	run, err := m.Hades.GetRun(ctx, id)
	if err != nil {
		return ErrSandboxNotFound
	}

	if err := m.Control.Exec(ctx, run.NodeID, id, cmd); err != nil {
		m.Logger.Error(ctx, "Failed to send exec command", map[string]any{
			"sandbox_id": id,
			"node_id":    run.NodeID,
			"error":      err,
		})
		return err
	}

	m.Logger.Info(ctx, "Exec command sent", map[string]any{
		"sandbox_id": id,
		"node_id":    run.NodeID,
		"command":    cmd,
	})
	return nil
}

// Reconcile rebuilds in-memory state by querying all nodes for running sandboxes.
func (m *Manager) Reconcile(ctx context.Context) error {
	m.Logger.Info(ctx, "Starting reconciliation", nil)

	// 1. List all nodes
	nodes, err := m.Hades.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, node := range nodes {
		// 2. Query each node for sandboxes
		runs, err := m.Control.ListSandboxes(ctx, node.ID)
		if err != nil {
			m.Logger.Error(ctx, "Failed to list sandboxes from node", map[string]any{
				"node_id": node.ID,
				"error":   err,
			})
			// Continue to next node, don't fail entire reconciliation
			continue
		}

		// 3. Update Hades
		for _, run := range runs {
			// Ensure node ID is set correctly (agent might not know its own ID in run struct?)
			// Agent returns SandboxRun, which has NodeID.
			// But let's enforce it matches the node we queried.
			run.NodeID = node.ID
			// Status should be RUNNING if it's in the list?
			// Runtime.List returns current state.

			if err := m.Hades.UpdateRun(ctx, run); err != nil {
				m.Logger.Error(ctx, "Failed to update run during reconciliation", map[string]any{
					"run_id":  run.ID,
					"node_id": node.ID,
					"error":   err,
				})
			}
		}
	}

	m.Logger.Info(ctx, "Reconciliation complete", nil)
	return nil
}
