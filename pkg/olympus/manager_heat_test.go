package olympus_test

import (
	"context"
	"testing"
	"time"

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

type mockLogger struct{}

func (m *mockLogger) Info(ctx context.Context, msg string, fields map[string]any)  {}
func (m *mockLogger) Error(ctx context.Context, msg string, fields map[string]any) {}

func TestManagerHeatClassification(t *testing.T) {
	// Setup dependencies
	queue := acheron.NewMemoryQueue()
	registry := hades.NewMemoryRegistry()
	policyRepo := themis.NewMemoryRepo()
	templateMgr := olympus.NewMemoryTemplateManager()
	metrics := hermes.NewNoopMetrics()
	logger := &mockLogger{}
	scheduler := moirai.NewLeastLoadedScheduler(logger)
	judgeChain := &judges.Chain{Pre: []judges.PreJudge{}}
	control := &olympus.NoopControlPlane{}
	heatClassifier := phlegethon.NewHeatClassifier()

	// Register a test node
	registry.UpdateHeartbeat(context.Background(), hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:      "test-node",
			Address: "127.0.0.1",
			Capacity: domain.ResourceCapacity{
				CPU: 8000,
				Mem: 16384,
			},
		},
		Load: domain.ResourceCapacity{
			CPU: 0,
			Mem: 0,
		},
		Time: time.Now(),
	})

	// Register a test template
	template := &domain.TemplateSpec{
		ID:          "test-template",
		Name:        "Test Template",
		Description: "A test template",
		BaseImage:   "/test/image.ext4",
		KernelImage: "/test/vmlinux",
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 512,
		},
	}
	templateMgr.RegisterTemplate(context.Background(), template)

	// Register a test policy
	policy := &domain.SandboxPolicy{
		ID:         "test-policy",
		TemplateID: "test-template",
		Resources: domain.ResourceSpec{
			CPU: 2000,
			Mem: 1024,
			TTL: 5 * time.Minute,
		},
		NetworkPolicy: domain.NetworkPolicyRef{
			ID:   "test-net",
			Name: "Test Network",
		},
	}
	policyRepo.UpsertPolicy(context.Background(), policy)

	manager := &olympus.Manager{
		Queue:      queue,
		Hades:      registry,
		Policies:   policyRepo,
		Templates:  templateMgr,
		Judges:     judgeChain,
		Scheduler:  scheduler,
		Phlegethon: heatClassifier,
		Control:    control,
		Metrics:    metrics,
		Logger:     logger,
	}

	t.Run("ColdWorkload", func(t *testing.T) {
		req := &domain.SandboxRequest{
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 500, // 0.5 cores
				Mem: 256, // 256 MB
				TTL: 10 * time.Second,
			},
		}

		err := manager.Submit(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if req.HeatLevel != string(phlegethon.HeatCold) {
			t.Errorf("expected heat level %s, got %s", phlegethon.HeatCold, req.HeatLevel)
		}
	})

	t.Run("WarmWorkload", func(t *testing.T) {
		req := &domain.SandboxRequest{
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1500, // 1.5 cores
				Mem: 1024, // 1 GB
				TTL: 2 * time.Minute,
			},
		}

		err := manager.Submit(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if req.HeatLevel != string(phlegethon.HeatWarm) {
			t.Errorf("expected heat level %s, got %s", phlegethon.HeatWarm, req.HeatLevel)
		}
	})

	t.Run("HotWorkload", func(t *testing.T) {
		req := &domain.SandboxRequest{
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 2500, // 2.5 cores
				Mem: 2048, // 2 GB
				TTL: 5 * time.Minute,
			},
		}

		err := manager.Submit(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if req.HeatLevel != string(phlegethon.HeatHot) {
			t.Errorf("expected heat level %s, got %s", phlegethon.HeatHot, req.HeatLevel)
		}
	})

	t.Run("InfernoWorkload", func(t *testing.T) {
		req := &domain.SandboxRequest{
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 4000, // 4 cores
				Mem: 8192, // 8 GB
				TTL: 30 * time.Minute,
			},
		}

		err := manager.Submit(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if req.HeatLevel != string(phlegethon.HeatInferno) {
			t.Errorf("expected heat level %s, got %s", phlegethon.HeatInferno, req.HeatLevel)
		}
	})

	t.Run("ExplicitHeatHint", func(t *testing.T) {
		req := &domain.SandboxRequest{
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 500, // Small resources
				Mem: 256,
				TTL: 10 * time.Second,
			},
			Metadata: map[string]string{
				"heat_hint": string(phlegethon.HeatInferno), // Override with hint
			},
		}

		err := manager.Submit(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Heat hint should override resource-based classification
		if req.HeatLevel != string(phlegethon.HeatInferno) {
			t.Errorf("expected heat level %s (from hint), got %s", phlegethon.HeatInferno, req.HeatLevel)
		}
	})

	t.Run("NilPhlegethonDoesNotCrash", func(t *testing.T) {
		// Create manager without Phlegethon
		managerNoHeat := &olympus.Manager{
			Queue:      queue,
			Hades:      registry,
			Policies:   policyRepo,
			Templates:  templateMgr,
			Judges:     judgeChain,
			Scheduler:  scheduler,
			Phlegethon: nil, // No heat classifier
			Control:    control,
			Metrics:    metrics,
			Logger:     logger,
		}

		req := &domain.SandboxRequest{
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
				TTL: 1 * time.Minute,
			},
		}

		err := managerNoHeat.Submit(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// HeatLevel should be empty when Phlegethon is nil
		if req.HeatLevel != "" {
			t.Errorf("expected empty heat level, got %s", req.HeatLevel)
		}
	})
}
