package moirai_test

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
)

type mockLogger struct{}

func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any)  {}
func (m *mockLogger) Error(ctx context.Context, msg string, fields map[string]any) {}

func TestScheduler(t *testing.T) {
	logger := &mockLogger{}

	nodes := []domain.NodeStatus{
		{
			NodeInfo: domain.NodeInfo{
				ID: "node-small-free",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"type": "cpu",
					"zone": "us-east-1a",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 7168, // 1024 MB free
			},
			Heartbeat: time.Now(),
		},
		{
			NodeInfo: domain.NodeInfo{
				ID: "node-large-free",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"type": "gpu",
					"zone": "us-east-1b",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 4096, // 4096 MB free
			},
			Heartbeat: time.Now(),
		},
		{
			NodeInfo: domain.NodeInfo{
				ID: "node-full",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 8192, // 0 MB free
			},
			Heartbeat: time.Now(),
		},
	}

	req := &domain.SandboxRequest{
		ID: "test-req",
		Resources: domain.ResourceSpec{
			Mem: 512,
		},
	}

	t.Run("LeastLoaded Strategy", func(t *testing.T) {
		s := moirai.NewScheduler("least-loaded", logger)
		nodeID, err := s.ChooseNode(context.Background(), req, nodes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nodeID != "node-large-free" {
			t.Errorf("expected node-large-free (most free mem), got %s", nodeID)
		}
	})

	t.Run("BinPacking Strategy", func(t *testing.T) {
		s := moirai.NewScheduler("bin-packing", logger)
		nodeID, err := s.ChooseNode(context.Background(), req, nodes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nodeID != "node-small-free" {
			t.Errorf("expected node-small-free (tightest fit), got %s", nodeID)
		}
	})

	t.Run("Affinity", func(t *testing.T) {
		// Prefer GPU node
		reqWithAffinity := &domain.SandboxRequest{
			ID: "test-req-affinity",
			Resources: domain.ResourceSpec{
				Mem: 512,
			},
			Metadata: map[string]string{
				"scheduler.affinity.type": "gpu",
			},
		}
		s := moirai.NewScheduler("least-loaded", logger)
		nodeID, err := s.ChooseNode(context.Background(), reqWithAffinity, nodes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nodeID != "node-large-free" {
			t.Errorf("expected node-large-free (affinity match), got %s", nodeID)
		}
	})

	t.Run("Anti-Affinity", func(t *testing.T) {
		// Avoid GPU node
		reqWithAntiAffinity := &domain.SandboxRequest{
			ID: "test-req-antiaffinity",
			Resources: domain.ResourceSpec{
				Mem: 512,
			},
			Metadata: map[string]string{
				"scheduler.antiaffinity.type": "gpu",
			},
		}
		s := moirai.NewScheduler("least-loaded", logger)
		nodeID, err := s.ChooseNode(context.Background(), reqWithAntiAffinity, nodes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nodeID != "node-small-free" {
			t.Errorf("expected node-small-free (anti-affinity match), got %s", nodeID)
		}
	})

	t.Run("No Capacity", func(t *testing.T) {
		hugeReq := &domain.SandboxRequest{
			ID: "test-req-huge",
			Resources: domain.ResourceSpec{
				Mem: 10000,
			},
		}
		s := moirai.NewScheduler("least-loaded", logger)
		_, err := s.ChooseNode(context.Background(), hugeReq, nodes)
		if err != moirai.ErrNoCapacity {
			t.Errorf("expected ErrNoCapacity, got %v", err)
		}
	})

	t.Run("QuarantineRouting_LeastLoaded", func(t *testing.T) {
		// Create nodes with one Typhon node
		typhonNode := domain.NodeStatus{
			NodeInfo: domain.NodeInfo{
				ID: "node-typhon",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"quarantine": "true",
					"role":       "typhon",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 2048, // 6144 MB free
			},
			Heartbeat: time.Now(),
		}

		regularNode := domain.NodeStatus{
			NodeInfo: domain.NodeInfo{
				ID: "node-regular",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"type": "standard",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 1024, // 7168 MB free (more free than typhon)
			},
			Heartbeat: time.Now(),
		}

		quarantineReq := &domain.SandboxRequest{
			ID: "test-quarantine-req",
			Resources: domain.ResourceSpec{
				Mem: 512,
			},
			Metadata: map[string]string{
				"quarantine": "true",
			},
		}

		s := moirai.NewScheduler("least-loaded", logger)
		nodeID, err := s.ChooseNode(context.Background(), quarantineReq, []domain.NodeStatus{typhonNode, regularNode})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nodeID != "node-typhon" {
			t.Errorf("expected quarantine request to route to node-typhon, got %s", nodeID)
		}
	})

	t.Run("QuarantineRouting_BinPacking", func(t *testing.T) {
		typhonNode1 := domain.NodeStatus{
			NodeInfo: domain.NodeInfo{
				ID: "node-typhon-1",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"quarantine": "true",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 2048, // 6144 MB free
			},
			Heartbeat: time.Now(),
		}

		typhonNode2 := domain.NodeStatus{
			NodeInfo: domain.NodeInfo{
				ID: "node-typhon-2",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"quarantine": "true",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 7168, // 1024 MB free (tighter fit)
			},
			Heartbeat: time.Now(),
		}

		quarantineReq := &domain.SandboxRequest{
			ID: "test-quarantine-binpack",
			Resources: domain.ResourceSpec{
				Mem: 512,
			},
			Metadata: map[string]string{
				"quarantine": "true",
			},
		}

		s := moirai.NewScheduler("bin-packing", logger)
		nodeID, err := s.ChooseNode(context.Background(), quarantineReq, []domain.NodeStatus{typhonNode1, typhonNode2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nodeID != "node-typhon-2" {
			t.Errorf("expected bin-packing to choose tighter fit (node-typhon-2), got %s", nodeID)
		}
	})

	t.Run("QuarantineFallback_NoTyphonNodes", func(t *testing.T) {
		// Only regular nodes, no Typhon nodes
		regularNodes := []domain.NodeStatus{
			{
				NodeInfo: domain.NodeInfo{
					ID: "node-regular-1",
					Capacity: domain.ResourceCapacity{
						Mem: 8192,
					},
					Labels: map[string]string{
						"type": "standard",
					},
				},
				Allocated: domain.ResourceCapacity{
					Mem: 1024,
				},
				Heartbeat: time.Now(),
			},
		}

		quarantineReq := &domain.SandboxRequest{
			ID: "test-quarantine-no-typhon",
			Resources: domain.ResourceSpec{
				Mem: 512,
			},
			Metadata: map[string]string{
				"quarantine": "true",
			},
		}

		s := moirai.NewScheduler("least-loaded", logger)
		_, err := s.ChooseNode(context.Background(), quarantineReq, regularNodes)
		if err != moirai.ErrNoTyphonNodes {
			t.Errorf("expected ErrNoTyphonNodes when no Typhon nodes available, got %v", err)
		}
	})

	t.Run("RegularRequest_IgnoresTyphonNodes", func(t *testing.T) {
		// Regular (non-quarantine) requests should prefer non-Typhon nodes
		typhonNode := domain.NodeStatus{
			NodeInfo: domain.NodeInfo{
				ID: "node-typhon",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"quarantine": "true",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 1024, // 7168 MB free
			},
			Heartbeat: time.Now(),
		}

		regularNode := domain.NodeStatus{
			NodeInfo: domain.NodeInfo{
				ID: "node-regular",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"type": "standard",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 2048, // 6144 MB free
			},
			Heartbeat: time.Now(),
		}

		regularReq := &domain.SandboxRequest{
			ID: "test-regular-req",
			Resources: domain.ResourceSpec{
				Mem: 512,
			},
			// No quarantine metadata
		}

		s := moirai.NewScheduler("least-loaded", logger)
		nodeID, err := s.ChooseNode(context.Background(), regularReq, []domain.NodeStatus{typhonNode, regularNode})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Least-loaded should pick the node with most free memory (typhon has more free)
		if nodeID != "node-typhon" {
			t.Errorf("expected least-loaded to pick node-typhon (most free), got %s", nodeID)
		}
	})

	t.Run("QuarantineWithAffinity", func(t *testing.T) {
		// Test quarantine + affinity combination
		typhonGPU := domain.NodeStatus{
			NodeInfo: domain.NodeInfo{
				ID: "node-typhon-gpu",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"quarantine": "true",
					"type":       "gpu",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 2048,
			},
			Heartbeat: time.Now(),
		}

		typhonCPU := domain.NodeStatus{
			NodeInfo: domain.NodeInfo{
				ID: "node-typhon-cpu",
				Capacity: domain.ResourceCapacity{
					Mem: 8192,
				},
				Labels: map[string]string{
					"quarantine": "true",
					"type":       "cpu",
				},
			},
			Allocated: domain.ResourceCapacity{
				Mem: 1024, // More free memory
			},
			Heartbeat: time.Now(),
		}

		quarantineGPUReq := &domain.SandboxRequest{
			ID: "test-quarantine-gpu",
			Resources: domain.ResourceSpec{
				Mem: 512,
			},
			Metadata: map[string]string{
				"quarantine":              "true",
				"scheduler.affinity.type": "gpu",
			},
		}

		s := moirai.NewScheduler("least-loaded", logger)
		nodeID, err := s.ChooseNode(context.Background(), quarantineGPUReq, []domain.NodeStatus{typhonGPU, typhonCPU})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nodeID != "node-typhon-gpu" {
			t.Errorf("expected quarantine request with GPU affinity to route to node-typhon-gpu, got %s", nodeID)
		}
	})
}
