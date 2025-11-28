package hades

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Registry tracks the underworld of nodes.

type Registry interface {
	ListNodes(ctx context.Context) ([]domain.NodeStatus, error)
	GetNode(ctx context.Context, id domain.NodeID) (*domain.NodeStatus, error)
	UpdateHeartbeat(ctx context.Context, payload HeartbeatPayload) error
	MarkDraining(ctx context.Context, id domain.NodeID) error
}

// HeartbeatPayload is what Hecatoncheir agents send periodically.

type HeartbeatPayload struct {
	Node domain.NodeInfo         `json:"node"`
	Load domain.ResourceCapacity `json:"load"`
	Time time.Time               `json:"time"`
}
