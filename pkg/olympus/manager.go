package olympus

import (
	"context"
	"fmt"

	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

// Manager is Olympus: front-door for users, back-door to Hades and Acheron.

type Manager struct {
	Queue     acheron.Queue
	Hades     hades.Registry
	Policies  themis.Repository
	Judges    *judges.Chain
	Scheduler moirai.Scheduler
	Control   ControlPlane
	Metrics   hermes.Metrics
	Logger    hermes.Logger
}

// Submit enqueues a new sandbox request after validation and policy checks.

func (m *Manager) Submit(ctx context.Context, req *domain.SandboxRequest) error {
	// 1) Load policy from Themis
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

	// 2) Run PreJudges
	verdict, err := m.Judges.RunPre(ctx, req)
	if err != nil {
		m.Logger.Error(ctx, "Judge evaluation failed", map[string]any{
			"sandbox_id": req.ID,
			"error":      err,
		})
		return err
	}

	if verdict != judges.VerdictAccept {
		m.Logger.Info(ctx, "Request rejected by policy enforcement", map[string]any{
			"sandbox_id": req.ID,
			"verdict":    verdict,
		})
		return fmt.Errorf("request rejected by policy enforcement")
	}

	m.Logger.Info(ctx, "Request passed all judges", map[string]any{
		"sandbox_id": req.ID,
	})

	// 3) Enqueue into Acheron
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
