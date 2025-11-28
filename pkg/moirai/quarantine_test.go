package moirai_test

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
)

func TestIsQuarantineRequest(t *testing.T) {
	tests := []struct {
		name     string
		req      *domain.SandboxRequest
		expected bool
	}{
		{
			name: "quarantine request",
			req: &domain.SandboxRequest{
				Metadata: map[string]string{
					"quarantine": "true",
				},
			},
			expected: true,
		},
		{
			name: "non-quarantine request",
			req: &domain.SandboxRequest{
				Metadata: map[string]string{
					"other": "value",
				},
			},
			expected: false,
		},
		{
			name:     "nil metadata",
			req:      &domain.SandboxRequest{},
			expected: false,
		},
		{
			name: "quarantine false",
			req: &domain.SandboxRequest{
				Metadata: map[string]string{
					"quarantine": "false",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := moirai.IsQuarantineRequest(tt.req)
			if result != tt.expected {
				t.Errorf("IsQuarantineRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilterTyphonNodes(t *testing.T) {
	typhonNode1 := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID: "typhon-1",
			Labels: map[string]string{
				"quarantine": "true",
			},
		},
	}

	typhonNode2 := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID: "typhon-2",
			Labels: map[string]string{
				"quarantine": "true",
				"zone":       "us-east-1a",
			},
		},
	}

	regularNode := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID: "regular-1",
			Labels: map[string]string{
				"type": "standard",
			},
		},
	}

	nodeWithWrongValue := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID: "wrong-value",
			Labels: map[string]string{
				"quarantine": "false",
			},
		},
	}

	nodeWithNoLabels := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID: "no-labels",
		},
	}

	tests := []struct {
		name          string
		nodes         []domain.NodeStatus
		expectedCount int
		expectedIDs   []string
	}{
		{
			name:          "all typhon nodes",
			nodes:         []domain.NodeStatus{typhonNode1, typhonNode2},
			expectedCount: 2,
			expectedIDs:   []string{"typhon-1", "typhon-2"},
		},
		{
			name:          "mixed nodes",
			nodes:         []domain.NodeStatus{typhonNode1, regularNode, typhonNode2},
			expectedCount: 2,
			expectedIDs:   []string{"typhon-1", "typhon-2"},
		},
		{
			name:          "no typhon nodes",
			nodes:         []domain.NodeStatus{regularNode, nodeWithNoLabels},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name:          "wrong quarantine value",
			nodes:         []domain.NodeStatus{nodeWithWrongValue},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name:          "empty input",
			nodes:         []domain.NodeStatus{},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := moirai.FilterTyphonNodes(tt.nodes)
			if len(result) != tt.expectedCount {
				t.Errorf("FilterTyphonNodes() returned %d nodes, want %d", len(result), tt.expectedCount)
			}

			resultIDs := make(map[string]bool)
			for _, node := range result {
				resultIDs[string(node.ID)] = true
			}

			for _, expectedID := range tt.expectedIDs {
				if !resultIDs[expectedID] {
					t.Errorf("Expected node %s not found in results", expectedID)
				}
			}
		})
	}
}

func TestQuarantineCapacityConstraints(t *testing.T) {
	logger := &mockLogger{}

	// Typhon node with insufficient capacity
	smallTyphonNode := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID: "typhon-small",
			Capacity: domain.ResourceCapacity{
				Mem: 1024,
			},
			Labels: map[string]string{
				"quarantine": "true",
			},
		},
		Allocated: domain.ResourceCapacity{
			Mem: 512, // 512 MB free
		},
		Heartbeat: time.Now(),
	}

	// Regular node with plenty of capacity
	largeRegularNode := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID: "regular-large",
			Capacity: domain.ResourceCapacity{
				Mem: 8192,
			},
			Labels: map[string]string{
				"type": "standard",
			},
		},
		Allocated: domain.ResourceCapacity{
			Mem: 1024, // 7168 MB free
		},
		Heartbeat: time.Now(),
	}

	// Quarantine request that needs more than small typhon node can provide
	largeQuarantineReq := &domain.SandboxRequest{
		ID: "large-quarantine",
		Resources: domain.ResourceSpec{
			Mem: 1024, // Needs 1024, but typhon node only has 512 free
		},
		Metadata: map[string]string{
			"quarantine": "true",
		},
	}

	s := moirai.NewScheduler("least-loaded", logger)
	_, err := s.ChooseNode(context.Background(), largeQuarantineReq, []domain.NodeStatus{smallTyphonNode, largeRegularNode})

	// Should fail with ErrNoCapacity (after filtering to Typhon nodes, none have capacity)
	if err != moirai.ErrNoCapacity {
		t.Errorf("expected ErrNoCapacity when Typhon node lacks capacity, got %v", err)
	}
}

func TestQuarantineUnhealthyNodes(t *testing.T) {
	logger := &mockLogger{}

	// Unhealthy Typhon node (stale heartbeat)
	unhealthyTyphon := domain.NodeStatus{
		NodeInfo: domain.NodeInfo{
			ID: "typhon-unhealthy",
			Capacity: domain.ResourceCapacity{
				Mem: 8192,
			},
			Labels: map[string]string{
				"quarantine": "true",
			},
		},
		Allocated: domain.ResourceCapacity{
			Mem: 2048,
		},
		Heartbeat: time.Now().Add(-30 * time.Second), // Stale heartbeat
	}

	quarantineReq := &domain.SandboxRequest{
		ID: "test-quarantine",
		Resources: domain.ResourceSpec{
			Mem: 512,
		},
		Metadata: map[string]string{
			"quarantine": "true",
		},
	}

	s := moirai.NewScheduler("least-loaded", logger)
	_, err := s.ChooseNode(context.Background(), quarantineReq, []domain.NodeStatus{unhealthyTyphon})

	// Should fail because the only Typhon node is unhealthy
	if err != moirai.ErrNoCapacity {
		t.Errorf("expected ErrNoCapacity when only Typhon node is unhealthy, got %v", err)
	}
}
