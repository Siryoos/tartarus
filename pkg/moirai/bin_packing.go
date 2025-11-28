package moirai

import (
	"context"
	"sort"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

type BinPackingScheduler struct {
	Logger hermes.Logger
}

func NewBinPackingScheduler(logger hermes.Logger) *BinPackingScheduler {
	return &BinPackingScheduler{
		Logger: logger,
	}
}

func (s *BinPackingScheduler) ChooseNode(ctx context.Context, req *domain.SandboxRequest, nodes []domain.NodeStatus) (domain.NodeID, error) {
	type candidate struct {
		node    domain.NodeStatus
		freeMem domain.Megabytes
	}

	var candidates []candidate

	now := time.Now()

	for _, node := range nodes {
		// 1. Filter Unhealthy Nodes (Heartbeat > 10s ago)
		if now.Sub(node.Heartbeat) > 10*time.Second {
			continue
		}

		// 2. Filter by Capacity
		freeMem := node.Capacity.Mem - node.Allocated.Mem
		if freeMem >= req.Resources.Mem {
			// 3. Filter by Affinity
			if CheckAffinity(req, node) {
				candidates = append(candidates, candidate{
					node:    node,
					freeMem: freeMem,
				})
			}
		}
	}

	if len(candidates) == 0 {
		return "", ErrNoCapacity
	}

	// 4. Sort by Available Memory (ASCENDING) - Tightest Fit
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].freeMem < candidates[j].freeMem
	})

	best := candidates[0]
	s.Logger.Info(ctx, "Scheduled sandbox (bin-packing)", map[string]any{
		"sandbox_id":  req.ID,
		"node_id":     best.node.ID,
		"free_mem_mb": best.freeMem,
	})

	return best.node.ID, nil
}
