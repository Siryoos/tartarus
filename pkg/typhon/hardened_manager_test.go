package typhon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

func TestHardenedQuarantineManager_Defaults(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{}
	classifier := &NoopClassifier{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	// Verify defaults are set
	assert.Equal(t, NetworkModeNone, hm.policy.DefaultNetworkMode, "Should default to no network")
	assert.Equal(t, SeccompQuarantine, hm.policy.Isolation.SeccompProfile, "Should default to quarantine seccomp")
	assert.NotNil(t, hm.policy.Isolation.StorageConfig, "Should have storage config")
	assert.True(t, hm.policy.Isolation.StorageConfig.UseSnapshotIsolation, "Should use snapshot isolation")
	assert.Equal(t, "quarantine:", hm.policy.Isolation.StorageConfig.SnapshotPrefix, "Should have quarantine prefix")
}

func TestHardenedQuarantineManager_Quarantine(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{
		DefaultNetworkMode: NetworkModeNone,
		Isolation: QuarantineIsolation{
			SeccompProfile: SeccompQuarantine,
		},
	}
	classifier := &NoopClassifier{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	ctx := context.Background()
	req := &QuarantineRequest{
		SandboxID:   "test-sandbox",
		Reason:      ReasonSuspiciousBehavior,
		RequestedBy: "admin",
		Evidence: []Evidence{
			{Type: EvidenceTypeNetworkLog, Description: "High egress"},
		},
		AutoQuarantine: false,
	}

	record, err := hm.Quarantine(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "test-sandbox", record.SandboxID)
	assert.Equal(t, ReasonSuspiciousBehavior, record.Reason)
	assert.Equal(t, StatusActive, record.Status)
}

func TestHardenedQuarantineManager_QuarantineAutoRequiresEvidence(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{}
	classifier := &NoopClassifier{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	ctx := context.Background()
	req := &QuarantineRequest{
		SandboxID:      "test-sandbox",
		Reason:         ReasonSuspiciousBehavior,
		RequestedBy:    "system",
		Evidence:       []Evidence{}, // No evidence
		AutoQuarantine: true,
	}

	_, err := hm.Quarantine(ctx, req)
	require.Error(t, err, "Auto-quarantine should require evidence")
	assert.Contains(t, err.Error(), "evidence")
}

func TestHardenedQuarantineManager_Release(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{}
	classifier := &NoopClassifier{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	ctx := context.Background()

	// First quarantine
	req := &QuarantineRequest{
		SandboxID:   "test-sandbox",
		Reason:      ReasonSuspiciousBehavior,
		RequestedBy: "admin",
		Evidence: []Evidence{
			{Type: EvidenceTypeNetworkLog, Description: "Test"},
		},
	}
	_, err := hm.Quarantine(ctx, req)
	require.NoError(t, err)

	// Then release
	approval := &ReleaseApproval{
		ApprovedBy: "admin",
		Reason:     "Verified safe",
	}

	err = hm.Release(ctx, "test-sandbox", approval)
	require.NoError(t, err)
}

func TestHardenedQuarantineManager_ReleaseWithNetworkOverride(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{}
	classifier := &NoopClassifier{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	ctx := context.Background()

	// First quarantine
	req := &QuarantineRequest{
		SandboxID:      "test-sandbox",
		Reason:         ReasonNetworkAnomaly,
		RequestedBy:    "admin",
		Evidence:       []Evidence{{Type: EvidenceTypeNetworkLog, Description: "Test"}},
		AutoQuarantine: false,
	}
	_, err := hm.Quarantine(ctx, req)
	require.NoError(t, err)

	// Release with network override
	approval := &ReleaseApproval{
		ApprovedBy: "admin",
		Reason:     "Need monitored network for analysis",
		NetworkOverride: &NetworkOverride{
			NetworkMode:   NetworkModeMonitored,
			AllowedEgress: []string{"analytics.example.com"},
			Justification: "Analysis requires external API",
		},
	}

	err = hm.Release(ctx, "test-sandbox", approval)
	require.NoError(t, err)
}

func TestHardenedQuarantineManager_ReleaseWithSecurityOverride(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{}
	classifier := &NoopClassifier{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	ctx := context.Background()

	// First quarantine
	req := &QuarantineRequest{
		SandboxID:   "test-sandbox",
		Reason:      ReasonSecurityScan,
		RequestedBy: "admin",
		Evidence:    []Evidence{{Type: EvidenceTypeSyscallTrace, Description: "Test"}},
	}
	_, err := hm.Quarantine(ctx, req)
	require.NoError(t, err)

	// Release with security override
	approval := &ReleaseApproval{
		ApprovedBy: "security-admin",
		Reason:     "Relaxed profile for debugging",
		SecurityOverride: &SecurityOverride{
			SeccompProfile: SeccompDefault,
			Justification:  "Need to debug syscall behavior",
		},
	}

	err = hm.Release(ctx, "test-sandbox", approval)
	require.NoError(t, err)
}

func TestHardenedQuarantineManager_Classify(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	// Create classifier with triggers
	triggers := []AutoQuarantineTrigger{
		{
			Condition: `cpu > 8000`,
			Reason:    ReasonResourceAbuse,
			Enabled:   true,
		},
	}
	classifier, err := NewRuleBasedClassifier(triggers)
	require.NoError(t, err)

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	ctx := context.Background()

	t.Run("TriggersQuarantine", func(t *testing.T) {
		sandbox := &domain.SandboxRequest{
			ID:       "high-cpu-sandbox",
			Template: "test",
			Resources: domain.ResourceSpec{
				CPU: 9000, // Exceeds trigger
				Mem: 512,
			},
		}

		shouldQuarantine, reason, evidence := hm.Classify(ctx, sandbox)
		assert.True(t, shouldQuarantine)
		assert.Equal(t, ReasonResourceAbuse, reason)
		assert.NotEmpty(t, evidence)
	})

	t.Run("DoesNotTrigger", func(t *testing.T) {
		sandbox := &domain.SandboxRequest{
			ID:       "normal-sandbox",
			Template: "test",
			Resources: domain.ResourceSpec{
				CPU: 1000, // Below trigger
				Mem: 512,
			},
		}

		shouldQuarantine, _, _ := hm.Classify(ctx, sandbox)
		assert.False(t, shouldQuarantine)
	})
}

func TestHardenedQuarantineManager_SetPolicy(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{}
	classifier := &NoopClassifier{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	ctx := context.Background()

	// Set new policy without defaults
	newPolicy := &QuarantinePolicy{
		QuarantineNodes: []string{"node-1", "node-2"},
	}

	err := hm.SetPolicy(ctx, newPolicy)
	require.NoError(t, err)

	// Verify defaults were enforced
	assert.Equal(t, NetworkModeNone, hm.policy.DefaultNetworkMode)
	assert.Equal(t, SeccompQuarantine, hm.policy.Isolation.SeccompProfile)
}

func TestHardenedQuarantineManager_GetIsolationConfig(t *testing.T) {
	base := NewInMemoryQuarantineManager()
	policy := &QuarantinePolicy{
		Isolation: QuarantineIsolation{
			NetworkMode:    NetworkModeNone,
			SeccompProfile: SeccompQuarantineStrict,
			StorageConfig: &QuarantineStorageConfig{
				IsolatedDir:          "/custom/quarantine",
				UseSnapshotIsolation: true,
				SnapshotPrefix:       "quar:",
			},
		},
	}
	classifier := &NoopClassifier{}
	logger := hermes.NewSlogAdapter()
	metrics := hermes.NewLogMetrics()

	hm := NewHardenedQuarantineManager(base, policy, classifier, logger, metrics)

	config := hm.GetIsolationConfig()
	assert.Equal(t, NetworkModeNone, config.NetworkMode)
	assert.Equal(t, SeccompQuarantineStrict, config.SeccompProfile)
	assert.Equal(t, "/custom/quarantine", config.StorageConfig.IsolatedDir)
	assert.Equal(t, "quar:", config.StorageConfig.SnapshotPrefix)
}
