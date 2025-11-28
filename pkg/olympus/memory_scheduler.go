package olympus

import (
	"context"
	"errors"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type MemoryScheduler struct{}

func NewMemoryScheduler() *MemoryScheduler {
	return &MemoryScheduler{}
}

func (s *MemoryScheduler) ChooseNode(ctx context.Context, req *domain.SandboxRequest, nodes []domain.NodeStatus) (domain.NodeID, error) {
	if len(nodes) == 0 {
		return "", errors.New("no nodes available")
	}
	// Simple first-available scheduler
	return nodes[0].ID, nil
}
