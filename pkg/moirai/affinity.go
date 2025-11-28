package moirai

import (
	"strings"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

const (
	AffinityPrefix     = "scheduler.affinity."
	AntiAffinityPrefix = "scheduler.antiaffinity."
)

// CheckAffinity returns true if the node satisfies all affinity and anti-affinity rules
// defined in the request metadata.
func CheckAffinity(req *domain.SandboxRequest, node domain.NodeStatus) bool {
	if req.Metadata == nil {
		return true
	}

	for key, value := range req.Metadata {
		// 1. Affinity: Node MUST have this label with this value
		if strings.HasPrefix(key, AffinityPrefix) {
			labelKey := strings.TrimPrefix(key, AffinityPrefix)
			nodeVal, ok := node.Labels[labelKey]
			if !ok || nodeVal != value {
				return false
			}
		}

		// 2. Anti-Affinity: Node MUST NOT have this label with this value
		// (It's okay if the label is missing, or present with a different value)
		if strings.HasPrefix(key, AntiAffinityPrefix) {
			labelKey := strings.TrimPrefix(key, AntiAffinityPrefix)
			nodeVal, ok := node.Labels[labelKey]
			if ok && nodeVal == value {
				return false
			}
		}
	}

	return true
}

// IsQuarantineRequest checks if the request requires quarantine isolation.
// A request is considered quarantined if it has metadata["quarantine"]="true".
func IsQuarantineRequest(req *domain.SandboxRequest) bool {
	if req.Metadata == nil {
		return false
	}
	return req.Metadata["quarantine"] == "true"
}

// FilterTyphonNodes returns only nodes suitable for quarantine workloads.
// Quarantine nodes must have the label "quarantine=true".
func FilterTyphonNodes(nodes []domain.NodeStatus) []domain.NodeStatus {
	var typhonNodes []domain.NodeStatus
	for _, node := range nodes {
		if node.Labels != nil && node.Labels["quarantine"] == "true" {
			typhonNodes = append(typhonNodes, node)
		}
	}
	return typhonNodes
}
