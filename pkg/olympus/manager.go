package olympus

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

var ErrPolicyRejected = errors.New("request rejected by policy enforcement")
var ErrSandboxNotFound = errors.New("sandbox not found")

// Manager is Olympus: front-door for users, back-door to Hades and Acheron.

type Manager struct {
	Queue     acheron.Queue
	Hades     hades.Registry
	Policies  themis.Repository
	Templates TemplateManager
	Judges    *judges.Chain
	Scheduler moirai.Scheduler
	Control   ControlPlane
	Metrics   hermes.Metrics
	Logger    hermes.Logger
}

// Submit enqueues a new sandbox request after validation and policy checks.

func (m *Manager) Submit(ctx context.Context, req *domain.SandboxRequest) error {
	// 1) Assign ID if missing
	if req.ID == "" {
		req.ID = domain.SandboxID(uuid.New().String())
	}

	// 2) Validate Template
	_, err := m.Templates.GetTemplate(ctx, req.Template)
	if err != nil {
		m.Logger.Error(ctx, "Template not found", map[string]any{
			"template": req.Template,
			"error":    err,
		})
		return fmt.Errorf("invalid template: %w", err)
	}

	// 3) Load policy from Themis
	policy, err := m.Policies.GetPolicy(ctx, req.Template)
	if err != nil {
		m.Logger.Error(ctx, "Failed to load policy", map[string]any{
			"template": req.Template,
			"error":    err,
		})
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
		return err
	}

	// 5) Verdict Handling
	switch verdict {
	case judges.VerdictReject:
		m.Logger.Info(ctx, "Request rejected by policy enforcement", map[string]any{
			"sandbox_id": req.ID,
			"verdict":    verdict,
		})
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

	// 6) Persistence (Optional/Future)
	// TODO: Persist request state to Hades/Redis

	// 7) Scheduling
	nodes, err := m.Hades.ListNodes(ctx)
	if err != nil {
		m.Logger.Error(ctx, "Failed to list nodes for scheduling", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	nodeID, err := m.Scheduler.ChooseNode(ctx, req, nodes)
	if err != nil {
		m.Logger.Error(ctx, "Failed to schedule sandbox", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		return fmt.Errorf("failed to schedule sandbox: %w", err)
	}
	req.NodeID = nodeID

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
		return err
	}

	m.Logger.Info(ctx, "Request successfully enqueued", map[string]any{
		"sandbox_id": req.ID,
	})
	return nil
}
