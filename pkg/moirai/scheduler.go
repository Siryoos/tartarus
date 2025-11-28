package moirai

import (
	"context"

	"errors"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

var ErrNoCapacity = errors.New("no nodes with sufficient capacity found")
var ErrNoTyphonNodes = errors.New("no typhon nodes available for quarantine workload")

// Scheduler chooses the fate of each sandbox: which node will host it.

type Scheduler interface {
	ChooseNode(ctx context.Context, req *domain.SandboxRequest, nodes []domain.NodeStatus) (domain.NodeID, error)
}

func NewScheduler(strategy string, logger hermes.Logger) Scheduler {
	switch strategy {
	case "bin-packing":
		return NewBinPackingScheduler(logger)
	case "least-loaded":
		return NewLeastLoadedScheduler(logger)
	default:
		logger.Info(context.Background(), "Unknown scheduler strategy, defaulting to least-loaded", map[string]any{"strategy": strategy})
		return NewLeastLoadedScheduler(logger)
	}
}
