package hades_test

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
)

func TestMemoryRegistry_NodeExpiration(t *testing.T) {
	registry := hades.NewMemoryRegistry()
	ctx := context.Background()

	// Create a heartbeat payload
	payload := hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:      "test-node-1",
			Address: "localhost",
			Labels:  map[string]string{"region": "us-west"},
			Capacity: domain.ResourceCapacity{
				CPU: 4000,
				Mem: 8192,
			},
		},
		Load: domain.ResourceCapacity{
			CPU: 1000,
			Mem: 2048,
		},
		Time: time.Now(),
	}

	// Add node
	err := registry.UpdateHeartbeat(ctx, payload)
	if err != nil {
		t.Fatalf("Failed to update heartbeat: %v", err)
	}

	// Verify node is in registry
	nodes, err := registry.ListNodes(ctx)
	if err != nil {
		t.Fatalf("Failed to list nodes: %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(nodes))
	}

	if nodes[0].ID != "test-node-1" {
		t.Errorf("Expected node ID test-node-1, got %s", nodes[0].ID)
	}

	// Verify node details
	node := nodes[0]
	if node.Capacity.CPU != 4000 {
		t.Errorf("Expected CPU capacity 4000, got %d", node.Capacity.CPU)
	}
	if node.Allocated.CPU != 1000 {
		t.Errorf("Expected allocated CPU 1000, got %d", node.Allocated.CPU)
	}

	t.Logf("✓ Node added successfully with capacity: CPU=%d, Mem=%d", node.Capacity.CPU, node.Capacity.Mem)
	t.Logf("✓ Node allocated resources: CPU=%d, Mem=%d", node.Allocated.CPU, node.Allocated.Mem)
}

func TestMemoryRegistry_NodeTTL(t *testing.T) {
	registry := hades.NewMemoryRegistry()
	ctx := context.Background()

	// Add node with old heartbeat
	payload := hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:      "expired-node",
			Address: "localhost",
			Capacity: domain.ResourceCapacity{
				CPU: 2000,
				Mem: 4096,
			},
		},
		Load: domain.ResourceCapacity{},
		Time: time.Now().Add(-35 * time.Second), // Expired (TTL is 30 seconds)
	}

	err := registry.UpdateHeartbeat(ctx, payload)
	if err != nil {
		t.Fatalf("Failed to update heartbeat: %v", err)
	}

	// Wait a small amount to ensure clock doesn't interfere
	time.Sleep(100 * time.Millisecond)

	// List nodes - expired node should be removed
	nodes, err := registry.ListNodes(ctx)
	if err != nil {
		t.Fatalf("Failed to list nodes: %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("Expected 0 nodes (expired), got %d", len(nodes))
	}

	t.Logf("✓ Expired nodes correctly filtered from ListNodes")

	// GetNode should also fail for expired node
	_, err = registry.GetNode(ctx, "expired-node")
	if err == nil {
		t.Error("Expected error when getting expired node, got nil")
	}

	t.Logf("✓ GetNode correctly rejects expired nodes")
}
