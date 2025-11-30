package typhon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestRuleBasedClassifier_CPUTrigger(t *testing.T) {
	triggers := []AutoQuarantineTrigger{
		{
			Condition: `cpu > 8000`,
			Reason:    ReasonResourceAbuse,
			Enabled:   true,
		},
	}

	classifier, err := NewRuleBasedClassifier(triggers)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("Triggers", func(t *testing.T) {
		sandbox := &domain.SandboxRequest{
			ID:       "high-cpu",
			Template: "test",
			Resources: domain.ResourceSpec{
				CPU: 9000,
				Mem: 512,
			},
		}

		shouldQuarantine, reason, evidence := classifier.ShouldQuarantine(ctx, sandbox)
		assert.True(t, shouldQuarantine)
		assert.Equal(t, ReasonResourceAbuse, reason)
		assert.NotEmpty(t, evidence)
	})

	t.Run("DoesNotTrigger", func(t *testing.T) {
		sandbox := &domain.SandboxRequest{
			ID:       "normal-cpu",
			Template: "test",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
		}

		shouldQuarantine, _, _ := classifier.ShouldQuarantine(ctx, sandbox)
		assert.False(t, shouldQuarantine)
	})
}

func TestRuleBasedClassifier_MemoryTrigger(t *testing.T) {
	triggers := []AutoQuarantineTrigger{
		{
			Condition: `mem > 16384`,
			Reason:    ReasonResourceAbuse,
			Enabled:   true,
		},
	}

	classifier, err := NewRuleBasedClassifier(triggers)
	require.NoError(t, err)

	ctx := context.Background()

	sandbox := &domain.SandboxRequest{
		ID:       "high-mem",
		Template: "test",
		Resources: domain.ResourceSpec{
			CPU: 1000,
			Mem: 20000,
		},
	}

	shouldQuarantine, reason, _ := classifier.ShouldQuarantine(ctx, sandbox)
	assert.True(t, shouldQuarantine)
	assert.Equal(t, ReasonResourceAbuse, reason)
}

func TestRuleBasedClassifier_MetadataTrigger(t *testing.T) {
	triggers := []AutoQuarantineTrigger{
		{
			Condition: `metadata["untrusted"] == "true"`,
			Reason:    ReasonUntrustedSource,
			Enabled:   true,
		},
	}

	classifier, err := NewRuleBasedClassifier(triggers)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("UntrustedSource", func(t *testing.T) {
		sandbox := &domain.SandboxRequest{
			ID:       "untrusted",
			Template: "test",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
			Metadata: map[string]string{
				"untrusted": "true",
			},
		}

		shouldQuarantine, reason, _ := classifier.ShouldQuarantine(ctx, sandbox)
		assert.True(t, shouldQuarantine)
		assert.Equal(t, ReasonUntrustedSource, reason)
	})

	t.Run("TrustedSource", func(t *testing.T) {
		sandbox := &domain.SandboxRequest{
			ID:       "trusted",
			Template: "test",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
			Metadata: map[string]string{
				"untrusted": "false",
			},
		}

		shouldQuarantine, _, _ := classifier.ShouldQuarantine(ctx, sandbox)
		assert.False(t, shouldQuarantine)
	})
}

func TestRuleBasedClassifier_DisabledTrigger(t *testing.T) {
	triggers := []AutoQuarantineTrigger{
		{
			Condition: `cpu > 1000`,
			Reason:    ReasonResourceAbuse,
			Enabled:   false, // Disabled
		},
	}

	classifier, err := NewRuleBasedClassifier(triggers)
	require.NoError(t, err)

	ctx := context.Background()

	sandbox := &domain.SandboxRequest{
		ID:       "high-cpu",
		Template: "test",
		Resources: domain.ResourceSpec{
			CPU: 9000,
			Mem: 512,
		},
	}

	shouldQuarantine, _, _ := classifier.ShouldQuarantine(ctx, sandbox)
	assert.False(t, shouldQuarantine, "Disabled trigger should not fire")
}

func TestRuleBasedClassifier_MultipleTriggers(t *testing.T) {
	triggers := []AutoQuarantineTrigger{
		{
			Condition: `cpu > 8000`,
			Reason:    ReasonResourceAbuse,
			Enabled:   true,
		},
		{
			Condition: `metadata["untrusted"] == "true"`,
			Reason:    ReasonUntrustedSource,
			Enabled:   true,
		},
	}

	classifier, err := NewRuleBasedClassifier(triggers)
	require.NoError(t, err)

	ctx := context.Background()

	// Should trigger on first matching rule
	sandbox := &domain.SandboxRequest{
		ID:       "multi-trigger",
		Template: "test",
		Resources: domain.ResourceSpec{
			CPU: 9000, // Triggers first rule
			Mem: 512,
		},
		Metadata: map[string]string{
			"untrusted": "true", // Would trigger second rule
		},
	}

	shouldQuarantine, reason, _ := classifier.ShouldQuarantine(ctx, sandbox)
	assert.True(t, shouldQuarantine)
	assert.Equal(t, ReasonResourceAbuse, reason, "Should use first matching trigger")
}

func TestGetDefaultTriggers(t *testing.T) {
	triggers := GetDefaultTriggers()

	assert.NotEmpty(t, triggers)

	// Verify we have common triggers
	hasResourceTrigger := false
	hasUntrustedTrigger := false
	hasNetworkTrigger := false

	for _, trigger := range triggers {
		if trigger.Reason == ReasonResourceAbuse {
			hasResourceTrigger = true
			assert.True(t, trigger.Enabled)
		}
		if trigger.Reason == ReasonUntrustedSource {
			hasUntrustedTrigger = true
			assert.True(t, trigger.Enabled)
		}
		if trigger.Reason == ReasonNetworkAnomaly {
			hasNetworkTrigger = true
			assert.True(t, trigger.Enabled)
		}
	}

	assert.True(t, hasResourceTrigger, "Should have resource abuse trigger")
	assert.True(t, hasUntrustedTrigger, "Should have untrusted source trigger")
	assert.True(t, hasNetworkTrigger, "Should have network anomaly trigger")
}

func TestNoopClassifier(t *testing.T) {
	classifier := &NoopClassifier{}
	ctx := context.Background()

	sandbox := &domain.SandboxRequest{
		ID:       "test",
		Template: "test",
		Resources: domain.ResourceSpec{
			CPU: 99999, // Extreme values
			Mem: 99999,
		},
	}

	shouldQuarantine, _, _ := classifier.ShouldQuarantine(ctx, sandbox)
	assert.False(t, shouldQuarantine, "Noop classifier should never quarantine")
}
