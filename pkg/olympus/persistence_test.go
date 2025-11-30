package olympus_test

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

func TestOlympusPersistence_RestartRecovery(t *testing.T) {
	// 1. Start MiniRedis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	// Helper to create a manager connected to the SAME Redis instance
	createManager := func() *olympus.Manager {
		// Registry
		reg, err := hades.NewRedisRegistry(mr.Addr(), 0, "")
		require.NoError(t, err)

		// Queue
		queue, err := acheron.NewRedisQueue(mr.Addr(), 0, "tartarus:queue", "", "", true, metrics, nil)
		require.NoError(t, err)

		// Control Plane
		// Note: Control plane initialization might need a real redis client if not using the interface
		// But for now, let's assume we just need Registry and Queue for this test as they hold state.
		// If ControlPlane is needed, we'd init it here too.

		// Dependencies
		policyRepo := themis.NewMemoryRepo()
		// Add a policy for our template
		policyRepo.UpsertPolicy(ctx, &domain.SandboxPolicy{
			ID:         "default-policy",
			TemplateID: "test-template",
			Resources:  domain.ResourceSpec{CPU: 100, Mem: 128},
			Retention:  domain.RetentionPolicy{MaxAge: time.Hour},
		})

		tplManager := olympus.NewMemoryTemplateManager()
		tplManager.RegisterTemplate(ctx, &domain.TemplateSpec{
			ID:        "test-template",
			Resources: domain.ResourceSpec{CPU: 100, Mem: 128},
		})

		scheduler := moirai.NewScheduler("least-loaded", logger)

		// Mock a node so scheduling succeeds
		reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
			Node: domain.NodeInfo{ID: "node-1", Capacity: domain.ResourceCapacity{CPU: 1000, Mem: 1000}},
			Time: time.Now(),
		})

		return &olympus.Manager{
			Queue:     queue,
			Hades:     reg,
			Policies:  policyRepo,
			Templates: tplManager,
			Judges:    &judges.Chain{Pre: []judges.PreJudge{}}, // Empty chain for simplicity
			Scheduler: scheduler,
			Control:   &olympus.NoopControlPlane{}, // We can use Noop for this test if we focus on Registry/Queue
			Metrics:   metrics,
			Logger:    logger,
		}
	}

	// 2. Initialize Manager (First Run)
	manager1 := createManager()

	// 3. Submit a Request
	reqID := domain.SandboxID(uuid.New().String())
	req := &domain.SandboxRequest{
		ID:       reqID,
		Template: "test-template",
	}

	err = manager1.Submit(ctx, req)
	require.NoError(t, err)

	// Verify it's in Registry (SCHEDULED or PENDING depending on scheduler speed, likely SCHEDULED)
	run1, err := manager1.Hades.GetRun(ctx, reqID)
	require.NoError(t, err)
	assert.Equal(t, reqID, run1.ID)
	assert.NotEmpty(t, run1.Status)

	// Verify it's in Queue (if status is not failed)
	// Queue inspection is harder with the interface, but we can trust Submit returned nil.

	// 4. "Restart" Manager (Create new instance connected to same Redis)
	manager2 := createManager()

	// 5. Verify State Survives
	run2, err := manager2.Hades.GetRun(ctx, reqID)
	require.NoError(t, err)
	assert.Equal(t, run1.ID, run2.ID)
	assert.Equal(t, run1.Status, run2.Status)
	assert.Equal(t, run1.NodeID, run2.NodeID)

	// Verify we can list it
	runs, err := manager2.Hades.ListRuns(ctx)
	require.NoError(t, err)
	found := false
	for _, r := range runs {
		if r.ID == reqID {
			found = true
			break
		}
	}
	assert.True(t, found, "Run should be listed after restart")
}
