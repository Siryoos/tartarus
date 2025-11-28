package moirai

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

func TestLeastLoadedScheduler_ChooseNode(t *testing.T) {
	logger := hermes.NewNoopLogger()
	scheduler := NewLeastLoadedScheduler(logger)
	ctx := context.Background()

	req := &domain.SandboxRequest{
		ID: "test-req",
		Resources: domain.ResourceSpec{
			Mem: 1024, // 1GB
		},
	}

	now := time.Now()

	tests := []struct {
		name     string
		nodes    []domain.NodeStatus
		wantNode domain.NodeID
		wantErr  error
	}{
		{
			name:    "No nodes",
			nodes:   []domain.NodeStatus{},
			wantErr: ErrNoCapacity,
		},
		{
			name: "Single healthy node with capacity",
			nodes: []domain.NodeStatus{
				{
					NodeInfo: domain.NodeInfo{
						ID: "node-1",
						Capacity: domain.ResourceCapacity{
							Mem: 4096,
						},
					},
					Allocated: domain.ResourceCapacity{
						Mem: 1024,
					},
					Heartbeat: now,
				},
			},
			wantNode: "node-1",
		},
		{
			name: "Single healthy node without capacity",
			nodes: []domain.NodeStatus{
				{
					NodeInfo: domain.NodeInfo{
						ID: "node-1",
						Capacity: domain.ResourceCapacity{
							Mem: 4096,
						},
					},
					Allocated: domain.ResourceCapacity{
						Mem: 3500, // Only 596MB free
					},
					Heartbeat: now,
				},
			},
			wantErr: ErrNoCapacity,
		},
		{
			name: "Unhealthy node ignored",
			nodes: []domain.NodeStatus{
				{
					NodeInfo: domain.NodeInfo{
						ID: "node-1",
						Capacity: domain.ResourceCapacity{
							Mem: 4096,
						},
					},
					Allocated: domain.ResourceCapacity{
						Mem: 0,
					},
					Heartbeat: now.Add(-20 * time.Second), // 20s ago
				},
			},
			wantErr: ErrNoCapacity,
		},
		{
			name: "Least loaded node chosen",
			nodes: []domain.NodeStatus{
				{
					NodeInfo: domain.NodeInfo{
						ID: "node-busy",
						Capacity: domain.ResourceCapacity{
							Mem: 4096,
						},
					},
					Allocated: domain.ResourceCapacity{
						Mem: 2048, // 2GB free
					},
					Heartbeat: now,
				},
				{
					NodeInfo: domain.NodeInfo{
						ID: "node-free",
						Capacity: domain.ResourceCapacity{
							Mem: 4096,
						},
					},
					Allocated: domain.ResourceCapacity{
						Mem: 0, // 4GB free
					},
					Heartbeat: now,
				},
			},
			wantNode: "node-free",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNode, err := scheduler.ChooseNode(ctx, req, tt.nodes)
			if err != tt.wantErr {
				t.Errorf("ChooseNode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotNode != tt.wantNode {
				t.Errorf("ChooseNode() = %v, want %v", gotNode, tt.wantNode)
			}
		})
	}
}
