package integration

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/acheron"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/erinyes"
	"github.com/tartarus-sandbox/tartarus/pkg/hades"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/judges"
	"github.com/tartarus-sandbox/tartarus/pkg/moirai"
	"github.com/tartarus-sandbox/tartarus/pkg/olympus"
	"github.com/tartarus-sandbox/tartarus/pkg/tartarus"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

// MockNetworkStatsProvider for testing
type MockNetworkStatsProvider struct {
	RxBytes   int64
	TxBytes   int64
	DropCount int
	Err       error
}

func (m *MockNetworkStatsProvider) GetInterfaceStats(ctx context.Context, ifaceName string) (int64, int64, error) {
	return m.RxBytes, m.TxBytes, m.Err
}

func (m *MockNetworkStatsProvider) GetDropCount(ctx context.Context, tapName string) (int, error) {
	return m.DropCount, m.Err
}

func TestPhase3Regression(t *testing.T) {
	// 1. Setup Infrastructure
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	// Helper to create Manager (for restart simulation)
	createManager := func() *olympus.Manager {
		// Registry
		reg, err := hades.NewRedisRegistry(mr.Addr(), 0, "")
		require.NoError(t, err)

		// Queue
		queue, err := acheron.NewRedisQueue(mr.Addr(), 0, "tartarus:queue", "group1", "consumer1", true, metrics)
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

		// Scheduler (Bin-Packing)
		scheduler := moirai.NewScheduler("bin-packing", logger)

		// Judges (Aeacus)
		aeacus := judges.NewAeacusJudge(logger)
		chain := &judges.Chain{
			Pre: []judges.PreJudge{aeacus},
		}

		return &olympus.Manager{
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
	}

	manager := createManager()

	// Register Nodes
	// Node 1: Small, GPU
	manager.Hades.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-gpu",
			Capacity: domain.ResourceCapacity{CPU: 1000, Mem: 2048},
			Labels:   map[string]string{"type": "gpu"},
		},
		Time: time.Now(),
	})
	// Node 2: Large, CPU (More free memory)
	manager.Hades.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-cpu",
			Capacity: domain.ResourceCapacity{CPU: 4000, Mem: 8192},
			Labels:   map[string]string{"type": "cpu"},
		},
		Time: time.Now(),
	})

	// 2. Submission & Audit (Aeacus)
	reqID := domain.SandboxID(uuid.New().String())
	req := &domain.SandboxRequest{
		ID:       reqID,
		Template: "test-template",
		Metadata: map[string]string{
			"scheduler.affinity.type": "gpu", // Force to GPU node despite it being smaller
		},
	}

	err = manager.Submit(ctx, req)
	require.NoError(t, err)

	// Verify Audit Metadata (Aeacus)
	// We verify this later when we dequeue, as Hades doesn't store the full request with metadata in SandboxRun.
	// But we can check that the run exists.
	run, err := manager.Hades.GetRun(ctx, reqID)
	require.NoError(t, err)
	assert.Equal(t, reqID, run.ID)

	// 3. Scheduling & Affinity (Moirai)
	// Submit calls Schedule internally.
	// Verify it landed on node-gpu
	assert.Equal(t, "node-gpu", string(run.NodeID), "Affinity should force scheduling to node-gpu")

	// 4. Queue Behavior (Acheron)
	// The request should be in the queue for node-gpu.
	// We need to consume from "tartarus:queue:node-gpu" because routing is enabled.
	// But our manager's queue is configured with "tartarus:queue".
	// Let's create a consumer specifically for that node's queue.
	nodeQueue, err := acheron.NewRedisQueue(mr.Addr(), 0, "tartarus:queue:node-gpu", "group1", "consumer1", false, metrics)
	require.NoError(t, err)

	// Dequeue
	dequeuedReq, receipt, err := nodeQueue.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, reqID, dequeuedReq.ID)

	// Verify Aeacus Metadata on the dequeued request
	assert.NotEmpty(t, dequeuedReq.Metadata["audit_id"], "Aeacus should add audit_id")
	assert.Equal(t, "standard", dequeuedReq.Metadata["compliance_level"], "Aeacus should add compliance_level")

	// Nack (Simulate failure)
	err = nodeQueue.Nack(ctx, receipt, "simulated failure")
	require.NoError(t, err)

	// Dequeue again (Redelivery)
	dequeuedReq2, receipt2, err := nodeQueue.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, reqID, dequeuedReq2.ID)
	assert.NotEqual(t, receipt, receipt2)

	// Ack (Success)
	err = nodeQueue.Ack(ctx, receipt2)
	require.NoError(t, err)

	// 5. Execution & Network Kill (Erinyes)
	// Simulate the agent picking it up and running it.
	// Update status to Running in Hades
	run.Status = domain.RunStatusRunning
	run.UpdatedAt = time.Now()
	err = manager.Hades.UpdateRun(ctx, *run)
	require.NoError(t, err)

	// Setup PollFury
	mockRuntime := tartarus.NewMockRuntime(slog.Default())
	mockNetStats := &MockNetworkStatsProvider{}
	fury := erinyes.NewPollFury(mockRuntime, logger, metrics, mockNetStats, 10*time.Millisecond)

	// Launch in MockRuntime to have it exist
	_, err = mockRuntime.Launch(ctx, dequeuedReq2, tartarus.VMConfig{
		TapDevice: "tap0",
	})
	require.NoError(t, err)

	// Arm Fury
	policy := &erinyes.PolicySnapshot{
		MaxNetworkEgressBytes: 1024 * 1024, // 1MB
		KillOnBreach:          true,
	}
	// Fetch the run again to get updated status
	runRunning, err := manager.Hades.GetRun(ctx, reqID)
	require.NoError(t, err)

	err = fury.Arm(ctx, runRunning, policy)
	require.NoError(t, err)

	// Simulate Network Breach
	mockNetStats.RxBytes = 2 * 1024 * 1024 // 2MB (Exceeds 1MB limit)

	// Wait for Fury to kill
	require.Eventually(t, func() bool {
		// Check MockRuntime status
		// MockRuntime deletes on kill, so Inspect returns error
		_, err := mockRuntime.Inspect(ctx, reqID)
		return err != nil
	}, 1*time.Second, 50*time.Millisecond, "Fury should kill the sandbox")

	// Update Hades status to Failed (Agent would do this)
	runRunning.Status = domain.RunStatusFailed
	runRunning.UpdatedAt = time.Now()
	err = manager.Hades.UpdateRun(ctx, *runRunning)
	require.NoError(t, err)

	// 6. Persistence Across Restarts (Olympus)
	// Re-create manager
	manager2 := createManager()

	// Verify state
	finalRun, err := manager2.Hades.GetRun(ctx, reqID)
	require.NoError(t, err)
	assert.Equal(t, domain.RunStatusFailed, finalRun.Status)
	assert.Equal(t, "node-gpu", string(finalRun.NodeID))
	// We cannot verify metadata in Hades as it's not stored in SandboxRun
}
