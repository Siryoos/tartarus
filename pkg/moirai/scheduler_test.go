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
}
