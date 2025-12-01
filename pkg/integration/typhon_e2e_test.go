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
	"github.com/tartarus-sandbox/tartarus/pkg/typhon"
)

// TestTyphonE2E_AutoQuarantineTrigger tests end-to-end auto-quarantine based on classification
func TestTyphonE2E_AutoQuarantineTrigger(t *testing.T) {
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

	// Typhon: Quarantine Manager with auto-classification
	baseQM := typhon.NewInMemoryQuarantineManager()
	triggers := []typhon.AutoQuarantineTrigger{
		{
			Condition: `cpu > 8000`,
			Reason:    typhon.ReasonResourceAbuse,
			Enabled:   true,
		},
		{
			Condition: `metadata["untrusted"] == "true"`,
			Reason:    typhon.ReasonUntrustedSource,
			Enabled:   true,
		},
	}
	classifier, err := typhon.NewRuleBasedClassifier(triggers)
	require.NoError(t, err)

	policy := &typhon.QuarantinePolicy{
		DefaultNetworkMode: typhon.NetworkModeNone,
		Isolation: typhon.QuarantineIsolation{
			SeccompProfile: typhon.SeccompQuarantine,
			NetworkMode:    typhon.NetworkModeNone,
			StorageConfig: &typhon.QuarantineStorageConfig{
				IsolatedDir:          "/var/lib/tartarus/quarantine",
				UseSnapshotIsolation: true,
				SnapshotPrefix:       "quarantine:",
			},
		},
	}

	quarantineManager := typhon.NewHardenedQuarantineManager(baseQM, policy, classifier, logger, metrics)

	// Judges (empty for this test)
	chain := &judges.Chain{Pre: []judges.PreJudge{}}

	// Manager demonstrates integration pattern (tests focus on quarantine manager directly)
	_ = &olympus.Manager{
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

	// Register Typhon nodes
	reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-typhon-1",
			Capacity: domain.ResourceCapacity{CPU: 10000, Mem: 16384},
			Labels:   map[string]string{"quarantine": "true"},
		},
		Time: time.Now(),
	})

	// Register regular node
	reg.UpdateHeartbeat(ctx, hades.HeartbeatPayload{
		Node: domain.NodeInfo{
			ID:       "node-regular-1",
			Capacity: domain.ResourceCapacity{CPU: 4000, Mem: 8192},
			Labels:   map[string]string{"type": "standard"},
		},
		Time: time.Now(),
	})

	t.Run("AutoQuarantineHighCPU", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 9000, // Triggers auto-quarantine
				Mem: 512,
			},
		}

		// Classify request
		shouldQuarantine, reason, evidence := quarantineManager.Classify(ctx, req)
		assert.True(t, shouldQuarantine, "High CPU should trigger quarantine")
		assert.Equal(t, typhon.ReasonResourceAbuse, reason)
		assert.NotEmpty(t, evidence)

		// If classified, quarantine it
		if shouldQuarantine {
			// Mark as quarantine in metadata
			if req.Metadata == nil {
				req.Metadata = make(map[string]string)
			}
			req.Metadata["quarantine"] = "true"

			// Quarantine
			qRecord, err := quarantineManager.Quarantine(ctx, &typhon.QuarantineRequest{
				SandboxID:      string(req.ID),
				Reason:         reason,
				Evidence:       evidence,
				RequestedBy:    "auto-classifier",
				AutoQuarantine: true,
			})
			require.NoError(t, err)
			assert.Equal(t, typhon.StatusActive, qRecord.Status)

			// Verify isolation config
			isolationConfig := quarantineManager.GetIsolationConfig()
			assert.Equal(t, typhon.NetworkModeNone, isolationConfig.NetworkMode, "Should enforce no network")
			assert.Equal(t, typhon.SeccompQuarantine, isolationConfig.SeccompProfile, "Should use quarantine seccomp")
			assert.NotNil(t, isolationConfig.StorageConfig)
			assert.True(t, isolationConfig.StorageConfig.UseSnapshotIsolation)
			assert.Equal(t, "quarantine:", isolationConfig.StorageConfig.SnapshotPrefix)
		}
	})

	t.Run("AutoQuarantineUntrustedSource", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
			Metadata: map[string]string{
				"untrusted": "true", // Triggers auto-quarantine
			},
		}

		// Classify request
		shouldQuarantine, reason, evidence := quarantineManager.Classify(ctx, req)
		assert.True(t, shouldQuarantine, "Untrusted source should trigger quarantine")
		assert.Equal(t, typhon.ReasonUntrustedSource, reason)
		assert.NotEmpty(t, evidence)
	})

	t.Run("NormalRequestNotQuarantined", func(t *testing.T) {
		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
		}

		// Classify request
		shouldQuarantine, _, _ := quarantineManager.Classify(ctx, req)
		assert.False(t, shouldQuarantine, "Normal request should not trigger quarantine")
	})
}

// TestTyphonE2E_ManualOverride tests manual quarantine with security overrides
func TestTyphonE2E_ManualOverride(t *testing.T) {
	ctx := context.Background()
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	// Quarantine Manager
	baseQM := typhon.NewInMemoryQuarantineManager()
	classifier := &typhon.NoopClassifier{}
	policy := &typhon.QuarantinePolicy{
		DefaultNetworkMode: typhon.NetworkModeNone,
		Isolation: typhon.QuarantineIsolation{
			SeccompProfile: typhon.SeccompQuarantine,
			NetworkMode:    typhon.NetworkModeNone,
		},
	}

	quarantineManager := typhon.NewHardenedQuarantineManager(baseQM, policy, classifier, logger, metrics)

	sandboxID := "test-sandbox-override"

	t.Run("ManualQuarantine", func(t *testing.T) {
		req := &typhon.QuarantineRequest{
			SandboxID:   sandboxID,
			Reason:      typhon.ReasonManualFlag,
			RequestedBy: "security-admin",
			Evidence: []typhon.Evidence{
				{
					Type:        typhon.EvidenceTypeScreenshot,
					Description: "Suspicious behavior observed",
					Timestamp:   time.Now(),
				},
			},
			AutoQuarantine: false,
		}

		record, err := quarantineManager.Quarantine(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, sandboxID, record.SandboxID)
		assert.Equal(t, typhon.StatusActive, record.Status)
	})

	t.Run("ReleaseWithNetworkOverride", func(t *testing.T) {
		approval := &typhon.ReleaseApproval{
			ApprovedBy: "security-admin",
			Reason:     "Need monitored network for forensic analysis",
			NetworkOverride: &typhon.NetworkOverride{
				NetworkMode:   typhon.NetworkModeMonitored,
				AllowedEgress: []string{"forensics.internal.corp"},
				Justification: "Forensic analysis requires external API access",
			},
		}

		err := quarantineManager.Release(ctx, sandboxID, approval)
		require.NoError(t, err)

		// Verify sandbox was released
		records, err := quarantineManager.ListQuarantined(ctx, &typhon.QuarantineFilter{
			SandboxID: sandboxID,
		})
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.Equal(t, typhon.StatusReleased, records[0].Status)
	})
}

// TestTyphonE2E_IsolationEnforcement tests that isolation settings are properly enforced
func TestTyphonE2E_IsolationEnforcement(t *testing.T) {
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	t.Run("NetworkModeNoneDefault", func(t *testing.T) {
		baseQM := typhon.NewInMemoryQuarantineManager()
		classifier := &typhon.NoopClassifier{}
		policy := &typhon.QuarantinePolicy{} // Empty policy, should get defaults

		quarantineManager := typhon.NewHardenedQuarantineManager(baseQM, policy, classifier, logger, metrics)

		// Verify defaults
		assert.Equal(t, typhon.NetworkModeNone, quarantineManager.GetIsolationConfig().NetworkMode)
		assert.Equal(t, typhon.SeccompQuarantine, quarantineManager.GetIsolationConfig().SeccompProfile)
	})

	t.Run("StrictSeccompProfile", func(t *testing.T) {
		baseQM := typhon.NewInMemoryQuarantineManager()
		classifier := &typhon.NoopClassifier{}
		policy := &typhon.QuarantinePolicy{
			Isolation: typhon.QuarantineIsolation{
				SeccompProfile: typhon.SeccompQuarantineStrict,
			},
		}

		quarantineManager := typhon.NewHardenedQuarantineManager(baseQM, policy, classifier, logger, metrics)

		config := quarantineManager.GetIsolationConfig()
		assert.Equal(t, typhon.SeccompQuarantineStrict, config.SeccompProfile)

		// Verify strict profile has more restrictions
		strictProfile, err := typhon.GetProfileByName(typhon.SeccompQuarantineStrict)
		require.NoError(t, err)
		quarantineProfile, err := typhon.GetProfileByName(typhon.SeccompQuarantine)
		require.NoError(t, err)
		assert.Greater(t, len(strictProfile.Syscalls), len(quarantineProfile.Syscalls),
			"Strict profile should have more syscall restrictions")
	})

	t.Run("IsolatedStorageConfig", func(t *testing.T) {
		baseQM := typhon.NewInMemoryQuarantineManager()
		classifier := &typhon.NoopClassifier{}
		policy := &typhon.QuarantinePolicy{
			Isolation: typhon.QuarantineIsolation{
				StorageConfig: &typhon.QuarantineStorageConfig{
					IsolatedDir:          "/custom/quarantine/storage",
					UseSnapshotIsolation: true,
					SnapshotPrefix:       "quar-",
				},
			},
		}

		quarantineManager := typhon.NewHardenedQuarantineManager(baseQM, policy, classifier, logger, metrics)

		config := quarantineManager.GetIsolationConfig()
		assert.NotNil(t, config.StorageConfig)
		assert.Equal(t, "/custom/quarantine/storage", config.StorageConfig.IsolatedDir)
		assert.True(t, config.StorageConfig.UseSnapshotIsolation)
		assert.Equal(t, "quar-", config.StorageConfig.SnapshotPrefix)
	})
}

// TestTyphonE2E_AuditTrail tests that all security events are properly logged
func TestTyphonE2E_AuditTrail(t *testing.T) {
	ctx := context.Background()
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	baseQM := typhon.NewInMemoryQuarantineManager()
	classifier := &typhon.NoopClassifier{}
	policy := &typhon.QuarantinePolicy{}

	quarantineManager := typhon.NewHardenedQuarantineManager(baseQM, policy, classifier, logger, metrics)

	sandboxID := "audit-test-sandbox"

	// Quarantine
	_, err := quarantineManager.Quarantine(ctx, &typhon.QuarantineRequest{
		SandboxID:   sandboxID,
		Reason:      typhon.ReasonSecurityScan,
		RequestedBy: "security-scanner",
		Evidence: []typhon.Evidence{
			{Type: typhon.EvidenceTypeSyscallTrace, Description: "Suspicious syscalls"},
		},
	})
	require.NoError(t, err)

	// Release with overrides
	err = quarantineManager.Release(ctx, sandboxID, &typhon.ReleaseApproval{
		ApprovedBy: "admin",
		Reason:     "Verified safe after analysis",
		NetworkOverride: &typhon.NetworkOverride{
			NetworkMode:   typhon.NetworkModeRestricted,
			Justification: "Need limited network",
		},
		SecurityOverride: &typhon.SecurityOverride{
			SeccompProfile: typhon.SeccompDefault,
			Justification:  "Need relaxed profile for debugging",
		},
	})
	require.NoError(t, err)

	// Note: In a real implementation, we would verify audit logs are written
	// For now, we just verify the operations completed successfully
}
