package olympus

import (
	"context"
	"io"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// ControlPlane handles command and control messages to agents.
type ControlPlane interface {
	Kill(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error
	StreamLogs(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, w io.Writer) error
	Hibernate(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error
	Wake(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error
}

// NoopControlPlane for when Redis is not available
type NoopControlPlane struct{}

func (n *NoopControlPlane) Kill(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}

func (n *NoopControlPlane) StreamLogs(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, w io.Writer) error {
	return nil
}

func (n *NoopControlPlane) Hibernate(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}

func (n *NoopControlPlane) Wake(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}
