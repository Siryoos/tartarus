package moirai

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Scheduler chooses the fate of each sandbox: which node will host it.

type Scheduler interface {
	ChooseNode(ctx context.Context, req *domain.SandboxRequest, nodes []domain.NodeStatus) (domain.NodeID, error)
}
