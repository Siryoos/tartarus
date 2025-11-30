package olympus

import (
	"context"
	"io"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// ControlPlane handles command and control messages to agents.
type ControlPlane interface {
	Kill(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error
	StreamLogs(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, w io.Writer, follow bool) error
	Hibernate(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error
	Wake(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error
	Snapshot(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error
	Exec(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, cmd []string) error
}

// NoopControlPlane for when Redis is not available
type NoopControlPlane struct{}

func (n *NoopControlPlane) Kill(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}

func (n *NoopControlPlane) StreamLogs(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, w io.Writer, follow bool) error {
	return nil
}

func (n *NoopControlPlane) Hibernate(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}

func (n *NoopControlPlane) Wake(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}

func (n *NoopControlPlane) Snapshot(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID) error {
	return nil
}

func (n *NoopControlPlane) Exec(ctx context.Context, nodeID domain.NodeID, sandboxID domain.SandboxID, cmd []string) error {
	return nil
}
