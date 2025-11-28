package moirai

import (
	"context"

	"errors"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

var ErrNoCapacity = errors.New("no nodes with sufficient capacity found")

// Scheduler chooses the fate of each sandbox: which node will host it.

type Scheduler interface {
	ChooseNode(ctx context.Context, req *domain.SandboxRequest, nodes []domain.NodeStatus) (domain.NodeID, error)
}
