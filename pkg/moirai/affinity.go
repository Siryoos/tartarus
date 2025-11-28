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
