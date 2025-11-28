package olympus

import (
	"context"

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
	Metrics   hermes.Metrics
	Logger    hermes.Logger
}

// Submit enqueues a new sandbox request after validation and policy checks.

func (m *Manager) Submit(ctx context.Context, req *domain.SandboxRequest) error {
	// 1) Load policy from Themis
	// 2) Run PreJudges
	// 3) Enqueue into Acheron
	return nil
}
