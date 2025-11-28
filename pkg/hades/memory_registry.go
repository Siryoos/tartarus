package hades

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

type MemoryRegistry struct {
	nodes sync.Map // map[domain.NodeID]domain.NodeStatus
}

func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{}
}

func (r *MemoryRegistry) ListNodes(ctx context.Context) ([]domain.NodeStatus, error) {
	var list []domain.NodeStatus
	r.nodes.Range(func(key, value any) bool {
		list = append(list, value.(domain.NodeStatus))
		return true
	})
	return list, nil
}

func (r *MemoryRegistry) GetNode(ctx context.Context, id domain.NodeID) (*domain.NodeStatus, error) {
	val, ok := r.nodes.Load(id)
	if !ok {
		return nil, errors.New("node not found")
	}
	status := val.(domain.NodeStatus)
	return &status, nil
}

func (r *MemoryRegistry) UpdateHeartbeat(ctx context.Context, status domain.NodeStatus) error {
	status.Heartbeat = time.Now()
	r.nodes.Store(status.ID, status)
	return nil
}

func (r *MemoryRegistry) MarkDraining(ctx context.Context, id domain.NodeID) error {
	val, ok := r.nodes.Load(id)
	if !ok {
		return errors.New("node not found")
	}
	status := val.(domain.NodeStatus)
	// In a real implementation, we'd have a state field.
	// For now, maybe we just log it or add a label?
	if status.Labels == nil {
		status.Labels = make(map[string]string)
	}
	status.Labels["status"] = "draining"
	r.nodes.Store(id, status)
	return nil
}
