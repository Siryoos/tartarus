package integration

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
	"github.com/tartarus-sandbox/tartarus/pkg/olympus"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

// TestPhase3QuarantinePlacement tests end-to-end quarantine placement with Typhon.
func TestPhase3QuarantinePlacement(t *testing.T) {
	// Setup infrastructure
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	// Registry
	reg, err := hades.NewRedisRegistry(mr.Addr(), 0, "")
	require.NoError(t, err)

	// Queue
	queue, err := acheron.NewRedisQueue(mr.Addr(), 0, "tartarus:queue", "group1", "consumer1", true, metrics, nil)
	require.NoError(t, err)

	// Policies
	policyRepo := themis.NewMemoryRepo()
	policyRepo.UpsertPolicy(ctx, &domain.SandboxPolicy{
		ID:         "default-policy",
		TemplateID: "test-template",
		Resources:  domain.ResourceSpec{CPU: 100, Mem: 128},
		Retention:  domain.RetentionPolicy{MaxAge: time.Hour},
		NetworkPolicy: domain.NetworkPolicyRef{
			Name: "default",
		},
	})

	// Templates
	tplManager := olympus.NewMemoryTemplateManager()
	tplManager.RegisterTemplate(ctx, &domain.TemplateSpec{
		ID:        "test-template",
		Resources: domain.ResourceSpec{CPU: 100, Mem: 128},
	})

	// Scheduler with quarantine support
	scheduler := moirai.NewScheduler("least-loaded", logger)

	// Judges (empty for this test)
	chain := &judges.Chain{Pre: []judges.PreJudge{}}

	// Manager
	manager := &olympus.Manager{
		Queue:     queue,
		Hades:     reg,
		Policies:  policyRepo,
		Templates: tplManager,
		Judges:    chain,
		Scheduler: scheduler,
		Control:   &olympus.NoopControlPlane{},
		Metrics:   metrics,
		Logger:    logger,
	}

	// Register nodes: 2 regular + 2 Typhon-labeled
	reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-regular-1",
			Capacity: domain.ResourceCapacity{CPU: 4000, Mem: 8192},
			Labels:   map[string]string{"type": "standard"},
		},
		Time: time.Now(),
	})

	reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-regular-2",
			Capacity: domain.ResourceCapacity{CPU: 4000, Mem: 8192},
			Labels:   map[string]string{"type": "standard"},
		},
		Time: time.Now(),
	})

	reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-typhon-1",
			Capacity: domain.ResourceCapacity{CPU: 2000, Mem: 4096},
			Labels:   map[string]string{"quarantine": "true", "zone": "typhon"},
		},
		Time: time.Now(),
	})

	reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-typhon-2",
			Capacity: domain.ResourceCapacity{CPU: 2000, Mem: 4096},
			Labels:   map[string]string{"quarantine": "true", "zone": "typhon"},
		},
		Time: time.Now(),
	})

	t.Run("QuarantineRequestRoutesToTyphonNode", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
			Metadata: map[string]string{
				"quarantine": "true", // Mark as quarantine
			},
		}

		err := manager.Submit(ctx, req)
		require.NoError(t, err)

		// Verify it was scheduled to a Typhon node
		run, err := manager.Hades.GetRun(ctx, req.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, run.NodeID)

		// Should be one of the Typhon nodes
		isTyphonNode := run.NodeID == "node-typhon-1" || run.NodeID == "node-typhon-2"
		assert.True(t, isTyphonNode, "Quarantine request should be scheduled to Typhon node, got %s", run.NodeID)
	})

	t.Run("NormalRequestAvoidsTyphonNodes", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
			// No quarantine metadata
		}

		err := manager.Submit(ctx, req)
		require.NoError(t, err)

		// Verify it was scheduled to a regular node
		run, err := manager.Hades.GetRun(ctx, req.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, run.NodeID)

		// Should NOT be a Typhon node
		isRegularNode := run.NodeID == "node-regular-1" || run.NodeID == "node-regular-2"
		assert.True(t, isRegularNode, "Normal request should avoid Typhon nodes, got %s", run.NodeID)
	})

	t.Run("QuarantineWithInsufficientTyphonCapacity", func(t *testing.T) {
		// Request resources larger than any single Typhon node
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 3000, // Exceeds Typhon node capacity (2000)
				Mem: 5000, // Exceeds Typhon node capacity (4096)
			},
			Metadata: map[string]string{
				"quarantine": "true",
			},
		}

		err := manager.Submit(ctx, req)
		// Should fail - no Typhon node has enough capacity
		assert.Error(t, err, "Should reject quarantine request when no Typhon node has capacity")
		assert.Contains(t, err.Error(), "no nodes with sufficient capacity", "Error should indicate no available nodes")
	})

	t.Run("MultipleQuarantineRequestsDistributed", func(t *testing.T) {
		// Submit multiple quarantine requests
		numRequests := 4
		requestIDs := make([]domain.SandboxID, numRequests)

		for i := 0; i < numRequests; i++ {
			req := &domain.SandboxRequest{
				ID:       domain.SandboxID(uuid.New().String()),
				Template: "test-template",
				Resources: domain.ResourceSpec{
					CPU: 500,
					Mem: 256,
				},
				Metadata: map[string]string{
					"quarantine": "true",
				},
			}
			requestIDs[i] = req.ID

			err := manager.Submit(ctx, req)
			require.NoError(t, err)
		}

		// Verify all were scheduled to Typhon nodes
		typhonNodeCounts := make(map[domain.NodeID]int)
		for _, reqID := range requestIDs {
			run, err := manager.Hades.GetRun(ctx, reqID)
			require.NoError(t, err)

			isTyphonNode := run.NodeID == "node-typhon-1" || run.NodeID == "node-typhon-2"
			assert.True(t, isTyphonNode, "All quarantine requests should go to Typhon nodes")

			typhonNodeCounts[run.NodeID]++
		}

		// Verify distribution across available Typhon nodes
		// With least-loaded scheduling, should be somewhat distributed
		assert.True(t, len(typhonNodeCounts) > 0, "Should use at least one Typhon node")
		assert.True(t, len(typhonNodeCounts) <= 2, "Should only use Typhon nodes")
	})

	t.Run("QuarantineWithStaleHeartbeat", func(t *testing.T) {
		// Update one Typhon node to have stale heartbeat
		reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
			Node: domain.NodeInfo{
				ID:       "node-typhon-1",
				Capacity: domain.ResourceCapacity{CPU: 2000, Mem: 4096},
				Labels:   map[string]string{"quarantine": "true"},
			},
			Time: time.Now().Add(-30 * time.Second), // Stale heartbeat
		})

		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
			Metadata: map[string]string{
				"quarantine": "true",
			},
		}

		err := manager.Submit(ctx, req)
		require.NoError(t, err)

		// Should route to the healthy Typhon node (node-typhon-2)
		run, err := manager.Hades.GetRun(ctx, req.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.NodeID("node-typhon-2"), run.NodeID, "Should route to healthy Typhon node")

		// Restore for other tests
		reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
			Node: domain.NodeInfo{
				ID:       "node-typhon-1",
				Capacity: domain.ResourceCapacity{CPU: 2000, Mem: 4096},
				Labels:   map[string]string{"quarantine": "true"},
			},
			Time: time.Now(),
		})
	})
}
