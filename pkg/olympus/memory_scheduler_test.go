package olympus_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/olympus"
)

func TestLeastLoadedScheduler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := olympus.NewMemoryScheduler(logger)
	ctx := context.Background()

	// Create test nodes with different resource availability
	nodes := []domain.NodeStatus{
		{
			NodeInfo: domain.NodeInfo{
				ID:      "node-1",
				Address: "10.0.0.1",
				Capacity: domain.ResourceCapacity{
					CPU: 8000,  // 8 cores
					Mem: 16384, // 16GB
				},
			},
			Allocated: domain.ResourceCapacity{
				CPU: 4000, // 4 cores used
				Mem: 8192, // 8GB used
			},
		},
		{
			NodeInfo: domain.NodeInfo{
				ID:      "node-2",
				Address: "10.0.0.2",
				Capacity: domain.ResourceCapacity{
					CPU: 8000,
					Mem: 16384,
				},
			},
			Allocated: domain.ResourceCapacity{
				CPU: 6000, // 6 cores used
				Mem: 4096, // 4GB used - MORE FREE RAM
			},
		},
		{
			NodeInfo: domain.NodeInfo{
				ID:      "node-3",
				Address: "10.0.0.3",
				Capacity: domain.ResourceCapacity{
					CPU: 4000,
					Mem: 8192,
				},
			},
			Allocated: domain.ResourceCapacity{
				CPU: 3000,
				Mem: 7000, // Very little free RAM
			},
		},
	}

	// Request that needs 2000 milliCPU and 4096 MB
	req := &domain.SandboxRequest{
		ID: "test-sandbox",
		Resources: domain.ResourceSpec{
			CPU: 2000,
			Mem: 4096,
		},
	}

	// Choose node
	nodeID, err := scheduler.ChooseNode(ctx, req, nodes)
	if err != nil {
		t.Fatalf("Failed to choose node: %v", err)
	}

	// Should choose node-2 because it has the most free RAM (12288 MB)
	// node-1 has 8192 MB free
	// node-2 has 12288 MB free ← winner
	// node-3 has 1192 MB free (not enough for request)
	if nodeID != "node-2" {
		t.Errorf("Expected node-2 (most free RAM), got %s", nodeID)
	}

	t.Logf("✓ Scheduler correctly chose node-2 with most free RAM")
}

func TestLeastLoadedScheduler_CPUTieBreaker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := olympus.NewMemoryScheduler(logger)
	ctx := context.Background()

	// Create nodes with SAME free RAM but different free CPU
	nodes := []domain.NodeStatus{
		{
			NodeInfo: domain.NodeInfo{
				ID: "node-1",
				Capacity: domain.ResourceCapacity{
					CPU: 8000,
					Mem: 16384,
				},
			},
			Allocated: domain.ResourceCapacity{
				CPU: 6000, // 2000 free
				Mem: 8192, // 8192 free
			},
		},
		{
			NodeInfo: domain.NodeInfo{
				ID: "node-2",
				Capacity: domain.ResourceCapacity{
					CPU: 8000,
					Mem: 16384,
				},
			},
			Allocated: domain.ResourceCapacity{
				CPU: 4000, // 4000 free ← more CPU
				Mem: 8192, // 8192 free (same as node-1)
			},
		},
	}

	req := &domain.SandboxRequest{
		ID: "test-sandbox",
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 2048,
		},
	}

	nodeID, err := scheduler.ChooseNode(ctx, req, nodes)
	if err != nil {
		t.Fatalf("Failed to choose node: %v", err)
	}

	// Should choose node-2 because same RAM but more free CPU
	if nodeID != "node-2" {
		t.Errorf("Expected node-2 (same RAM, more CPU), got %s", nodeID)
	}

	t.Logf("✓ Scheduler correctly used CPU as tie-breaker")
}

func TestLeastLoadedScheduler_NoViableNode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := olympus.NewMemoryScheduler(logger)
	ctx := context.Background()

	// Create nodes with insufficient resources
	nodes := []domain.NodeStatus{
		{
			NodeInfo: domain.NodeInfo{
				ID: "node-1",
				Capacity: domain.ResourceCapacity{
					CPU: 2000,
					Mem: 4096,
				},
			},
			Allocated: domain.ResourceCapacity{
				CPU: 1000,
				Mem: 2048,
			},
		},
	}

	// Request needs more than available
	req := &domain.SandboxRequest{
		ID: "test-sandbox",
		Resources: domain.ResourceSpec{
			CPU: 5000, // More than node has
			Mem: 8192,
		},
	}

	_, err := scheduler.ChooseNode(ctx, req, nodes)
	if err == nil {
		t.Error("Expected error for insufficient resources, got nil")
	}

	t.Logf("✓ Scheduler correctly rejects when no viable nodes: %v", err)
}
