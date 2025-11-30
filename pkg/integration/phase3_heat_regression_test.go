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
	"github.com/tartarus-sandbox/tartarus/pkg/phlegethon"
	"github.com/tartarus-sandbox/tartarus/pkg/themis"
)

// TestPhase3HeatAwareScheduling tests end-to-end heat-aware scheduling with Phlegethon.
func TestPhase3HeatAwareScheduling(t *testing.T) {
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

	// Scheduler
	scheduler := moirai.NewScheduler("least-loaded", logger)

	// Judges
	chain := &judges.Chain{Pre: []judges.PreJudge{}}

	// Phlegethon heat classifier
	heatClassifier := phlegethon.NewHeatClassifier()

	// Manager with Phlegethon
	manager := &olympus.Manager{
		Queue:      queue,
		Hades:      reg,
		Policies:   policyRepo,
		Templates:  tplManager,
		Judges:     chain,
		Scheduler:  scheduler,
		Phlegethon: heatClassifier,
		Control:    &olympus.NoopControlPlane{},
		Metrics:    metrics,
		Logger:     logger,
	}

	// Register nodes with different capacities
	// Small node - good for cold workloads
	reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-small",
			Capacity: domain.ResourceCapacity{CPU: 2000, Mem: 2048},
			Labels:   map[string]string{"size": "small"},
		},
		Time: time.Now(),
	})

	// Large node - good for hot workloads
	reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-large",
			Capacity: domain.ResourceCapacity{CPU: 8000, Mem: 16384},
			Labels:   map[string]string{"size": "large"},
		},
		Time: time.Now(),
	})

	t.Run("ColdWorkloadClassification", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 500, // 0.5 cores
				Mem: 256, // 256 MB
				TTL: 10 * time.Second,
			},
		}

		err := manager.Submit(ctx, req)
		require.NoError(t, err)

		// Verify heat level was set to cold
		assert.Equal(t, string(phlegethon.HeatCold), req.HeatLevel, "Small, short-lived workload should be classified as COLD")

		// Verify it was scheduled
		run, err := manager.Hades.GetRun(ctx, req.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, run.NodeID)
	})

	t.Run("HotWorkloadClassification", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 2500, // 2.5 cores
				Mem: 2048, // 2 GB
				TTL: 5 * time.Minute,
			},
		}

		err := manager.Submit(ctx, req)
		require.NoError(t, err)

		// Verify heat level was set to hot
		assert.Equal(t, string(phlegethon.HeatHot), req.HeatLevel, "Large, medium-lived workload should be classified as HOT")

		// Verify it was scheduled
		run, err := manager.Hades.GetRun(ctx, req.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, run.NodeID)
	})

	t.Run("InfernoWorkloadClassification", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 4000, // 4 cores
				Mem: 8192, // 8 GB
				TTL: 30 * time.Minute,
			},
		}

		err := manager.Submit(ctx, req)
		require.NoError(t, err)

		// Verify heat level was set to inferno
		assert.Equal(t, string(phlegethon.HeatInferno), req.HeatLevel, "Very large, long-lived workload should be classified as INFERNO")

		// Verify it was scheduled
		run, err := manager.Hades.GetRun(ctx, req.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, run.NodeID)
	})

	t.Run("HeatHintOverride", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 500, // Small resources
				Mem: 256,
				TTL: 10 * time.Second,
			},
			Metadata: map[string]string{
				"heat_hint": string(phlegethon.HeatInferno), // Override to inferno
			},
		}

		err := manager.Submit(ctx, req)
		require.NoError(t, err)

		// Verify heat hint was honored
		assert.Equal(t, string(phlegethon.HeatInferno), req.HeatLevel, "Heat hint should override resource-based classification")

		// Verify it was scheduled
		run, err := manager.Hades.GetRun(ctx, req.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, run.NodeID)
	})
}
