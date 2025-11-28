package olympus

import (
	"context"
	"errors"
	"log/slog"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type MemoryScheduler struct {
	Logger *slog.Logger
}

func NewMemoryScheduler(logger *slog.Logger) *MemoryScheduler {
	return &MemoryScheduler{
		Logger: logger,
	}
}

func (s *MemoryScheduler) ChooseNode(ctx context.Context, req *domain.SandboxRequest, nodes []domain.NodeStatus) (domain.NodeID, error) {
	if len(nodes) == 0 {
		return "", errors.New("no nodes available")
	}

	type candidate struct {
		nodeID  domain.NodeID
		freeMem domain.Megabytes
		freeCPU domain.MilliCPU
	}

	var eligible []candidate

	// Filter nodes that can satisfy the request
	for _, node := range nodes {
		freeMem := node.Capacity.Mem - node.Allocated.Mem
		freeCPU := node.Capacity.CPU - node.Allocated.CPU

		// Check if node has sufficient resources
		if freeMem >= req.Resources.Mem && freeCPU >= req.Resources.CPU {
			eligible = append(eligible, candidate{
				nodeID:  node.ID,
				freeMem: freeMem,
				freeCPU: freeCPU,
			})
		}
	}

	if len(eligible) == 0 {
		s.Logger.Error("No nodes can satisfy resource requirements",
			"required_mem_mb", req.Resources.Mem,
			"required_cpu_milli", req.Resources.CPU)
		return "", errors.New("no nodes can satisfy resource requirements")
	}

	// Choose node with most free RAM, breaking ties with most free CPU
	best := eligible[0]
	for _, c := range eligible[1:] {
		if c.freeMem > best.freeMem || (c.freeMem == best.freeMem && c.freeCPU > best.freeCPU) {
			best = c
		}
	}

	s.Logger.Info("Scheduled sandbox",
		"sandbox_id", req.ID,
		"node_id", best.nodeID,
		"free_mem_mb", best.freeMem,
		"free_cpu_milli", best.freeCPU,
		"required_mem_mb", req.Resources.Mem,
		"required_cpu_milli", req.Resources.CPU)

	return best.nodeID, nil
}
