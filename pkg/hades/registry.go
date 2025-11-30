package hades

import (
	"context"
	"errors"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

var (
	ErrNodeNotFound = errors.New("node not found")
	ErrRunNotFound  = errors.New("run not found")
)

// Registry tracks the underworld of nodes.

type Registry interface {
	ListNodes(ctx context.Context) ([]domain.NodeStatus, error)
	GetNode(ctx context.Context, id domain.NodeID) (*domain.NodeStatus, error)
	UpdateHeartbeat(ctx context.Context, payload HeartbeatPayload) error
	MarkDraining(ctx context.Context, id domain.NodeID) error

	// Run persistence
	UpdateRun(ctx context.Context, run domain.SandboxRun) error
	GetRun(ctx context.Context, id domain.SandboxID) (*domain.SandboxRun, error)
	ListRuns(ctx context.Context) ([]domain.SandboxRun, error)
}

// HeartbeatPayload is what Hecatoncheir agents send periodically.

type HeartbeatPayload struct {
	Node            domain.NodeInfo         `json:"node"`
	Load            domain.ResourceCapacity `json:"load"`
	ActiveSandboxes []domain.SandboxRun     `json:"active_sandboxes"`
	Time            time.Time               `json:"time"`
}
