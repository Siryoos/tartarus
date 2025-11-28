package olympus

import (
	"context"
	"fmt"
	"io"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// ListSandboxes aggregates active sandboxes from all nodes.
func (m *Manager) ListSandboxes(ctx context.Context) ([]domain.SandboxRun, error) {
	nodes, err := m.Hades.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	var allRuns []domain.SandboxRun
	for _, node := range nodes {
		allRuns = append(allRuns, node.ActiveSandboxes...)
	}
	return allRuns, nil
}

// KillSandbox terminates a sandbox.
func (m *Manager) KillSandbox(ctx context.Context, id domain.SandboxID) error {
	// Find which node has the sandbox
	nodes, err := m.Hades.ListNodes(ctx)
	if err != nil {
		return err
	}

	var targetNode domain.NodeID
	found := false
	for _, node := range nodes {
		for _, run := range node.ActiveSandboxes {
			if run.ID == id || run.RequestID == id { // Handle both run ID and request ID if they differ
				targetNode = node.ID
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("sandbox not found")
	}

	return m.Control.Kill(ctx, targetNode, id)
}

// StreamLogs streams logs from a sandbox.
func (m *Manager) StreamLogs(ctx context.Context, id domain.SandboxID, w io.Writer) error {
	// Find which node has the sandbox
	nodes, err := m.Hades.ListNodes(ctx)
	if err != nil {
		return err
	}

	var targetNode domain.NodeID
	found := false
	for _, node := range nodes {
		for _, run := range node.ActiveSandboxes {
			if run.ID == id || run.RequestID == id {
				targetNode = node.ID
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("sandbox not found")
	}

	return m.Control.StreamLogs(ctx, targetNode, id, w)
}
