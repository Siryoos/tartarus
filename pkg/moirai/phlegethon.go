package moirai

import (
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/phlegethon"
)

const PoolLabel = "phlegethon.tartarus.io/pool"

// FilterPhlegethonNodes filters nodes based on resource class requirements and pool labels.
func FilterPhlegethonNodes(nodes []domain.NodeStatus, heatLevel string) []domain.NodeStatus {
	if heatLevel == "" {
		return nodes
	}

	// Resolve resource class from heat level
	hl := phlegethon.HeatLevel(heatLevel)
	class, ok := phlegethon.DefaultResourceClasses[hl]
	if !ok {
		// Unknown heat level, return all nodes (or maybe log warning?)
		return nodes
	}

	var filtered []domain.NodeStatus
	for _, node := range nodes {
		// 1. GPU Requirement (Critical for Inferno)
		if class.GPUCount > 0 {
			if node.Capacity.GPU < class.GPUCount {
				continue
			}
		}

		// 2. Pool Label Enforcement
		// If a node is labeled for a specific pool, it can only host workloads of that class.
		// If a node is unlabeled, it can host any workload (subject to capacity).
		if pool, ok := node.Labels[PoolLabel]; ok {
			if pool != class.Name {
				continue
			}
		}

		filtered = append(filtered, node)
	}

	return filtered
}
