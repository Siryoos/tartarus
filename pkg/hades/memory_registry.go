package hades

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

const (
	// NodeTTL is the maximum time since last heartbeat before a node is considered dead
	NodeTTL = 30 * time.Second
)

type MemoryRegistry struct {
	nodes sync.Map // map[domain.NodeID]domain.NodeStatus
	runs  sync.Map // map[domain.SandboxID]domain.SandboxRun
}

func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{}
}

func (r *MemoryRegistry) ListNodes(ctx context.Context) ([]domain.NodeStatus, error) {
	var list []domain.NodeStatus
	now := time.Now()

	r.nodes.Range(func(key, value any) bool {
		nodeID := key.(domain.NodeID)
		status := value.(domain.NodeStatus)

		// Check if node has expired
		if now.Sub(status.Heartbeat) > NodeTTL {
			// Remove expired node
			r.nodes.Delete(nodeID)
			return true // continue iteration
		}

		list = append(list, status)
		return true
	})

	return list, nil
}

func (r *MemoryRegistry) GetNode(ctx context.Context, id domain.NodeID) (*domain.NodeStatus, error) {
	val, ok := r.nodes.Load(id)
	if !ok {
		return nil, ErrNodeNotFound
	}
	status := val.(domain.NodeStatus)

	// Check if node has expired
	if time.Since(status.Heartbeat) > NodeTTL {
		r.nodes.Delete(id)
		return nil, errors.New("node expired")
	}

	return &status, nil
}

func (r *MemoryRegistry) UpdateHeartbeat(ctx context.Context, payload HeartbeatPayload) error {
	// Build NodeStatus from HeartbeatPayload
	status := domain.NodeStatus{
		NodeInfo:        payload.Node,
		Allocated:       payload.Load,
		ActiveSandboxes: payload.ActiveSandboxes,
		Heartbeat:       payload.Time,
	}

	r.nodes.Store(status.ID, status)
	return nil
}

func (r *MemoryRegistry) MarkDraining(ctx context.Context, id domain.NodeID) error {
	val, ok := r.nodes.Load(id)
	if !ok {
		return errors.New("node not found")
	}
	status := val.(domain.NodeStatus)

	// Initialize Labels map if needed
	if status.Labels == nil {
		status.Labels = make(map[string]string)
	}
	status.Labels["status"] = "draining"
	r.nodes.Store(id, status)
	return nil
}

func (r *MemoryRegistry) UpdateRun(ctx context.Context, run domain.SandboxRun) error {
	r.runs.Store(run.ID, run)
	return nil
}

func (r *MemoryRegistry) GetRun(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error) {
	val, ok := r.runs.Load(id)
	if !ok {
		return nil, ErrRunNotFound
	}
	run := val.(domain.SandboxRun)
	return &run, nil
}

func (r *MemoryRegistry) ListRuns(ctx context.Context) ([]domain.SandboxRun, error) {
	var list []domain.SandboxRun
	r.runs.Range(func(key, value any) bool {
		run := value.(domain.SandboxRun)
		list = append(list, run)
		return true
	})
	return list, nil
}
