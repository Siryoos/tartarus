package thanatos

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestGracePolicy_EffectiveGrace(t *testing.T) {
	tests := []struct {
		name      string
		policy    *GracePolicy
		requested time.Duration
		expected  time.Duration
	}{
		{
			name: "uses requested when valid",
			policy: &GracePolicy{
				DefaultGrace: 5 * time.Second,
				MaxGrace:     30 * time.Second,
			},
			requested: 10 * time.Second,
			expected:  10 * time.Second,
		},
		{
			name: "uses default when requested is zero",
			policy: &GracePolicy{
				DefaultGrace: 5 * time.Second,
				MaxGrace:     30 * time.Second,
			},
			requested: 0,
			expected:  5 * time.Second,
		},
		{
			name: "falls back to DefaultGracePeriod when both zero",
			policy: &GracePolicy{
				DefaultGrace: 0,
				MaxGrace:     30 * time.Second,
			},
			requested: 0,
			expected:  DefaultGracePeriod,
		},
		{
			name: "clamps to MaxGrace",
			policy: &GracePolicy{
				DefaultGrace: 5 * time.Second,
				MaxGrace:     10 * time.Second,
			},
			requested: 20 * time.Second,
			expected:  10 * time.Second,
		},
		{
			name: "clamps to MaxGracePeriod when no policy max",
			policy: &GracePolicy{
				DefaultGrace: 5 * time.Second,
				MaxGrace:     0,
			},
			requested: 10 * time.Minute,
			expected:  MaxGracePeriod,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.policy.EffectiveGrace(tt.requested)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGracePolicy_Clone(t *testing.T) {
	original := &GracePolicy{
		ID:              "test-policy",
		Name:            "Test Policy",
		DefaultGrace:    10 * time.Second,
		MaxGrace:        60 * time.Second,
		CheckpointFirst: true,
		ExportLogs:      true,
		ExportArtifacts: false,
	}

	clone := original.Clone()
	require.NotNil(t, clone)
	require.Equal(t, original.ID, clone.ID)
	require.Equal(t, original.Name, clone.Name)
	require.Equal(t, original.DefaultGrace, clone.DefaultGrace)
	require.Equal(t, original.MaxGrace, clone.MaxGrace)
	require.Equal(t, original.CheckpointFirst, clone.CheckpointFirst)
	require.Equal(t, original.ExportLogs, clone.ExportLogs)
	require.Equal(t, original.ExportArtifacts, clone.ExportArtifacts)

	// Ensure clone is independent
	clone.Name = "Modified"
	require.NotEqual(t, original.Name, clone.Name)
}

func TestStaticPolicyResolver_ResolvesDefault(t *testing.T) {
	defaultPolicy := &GracePolicy{
		ID:           "default",
		Name:         "Default",
		DefaultGrace: 5 * time.Second,
	}
	resolver := NewStaticPolicyResolver(defaultPolicy)
	ctx := context.Background()

	policy, err := resolver.ResolvePolicy(ctx, "sandbox-1", "unknown-template", ReasonUserRequest)
	require.NoError(t, err)
	require.NotNil(t, policy)
	require.Equal(t, "default", policy.ID)
}

func TestStaticPolicyResolver_ResolvesTemplatePolicy(t *testing.T) {
	defaultPolicy := &GracePolicy{
		ID:           "default",
		Name:         "Default",
		DefaultGrace: 5 * time.Second,
	}
	templatePolicy := &GracePolicy{
		ID:              "python-template",
		Name:            "Python Template Policy",
		DefaultGrace:    15 * time.Second,
		CheckpointFirst: true,
	}

	resolver := NewStaticPolicyResolver(defaultPolicy)
	resolver.SetTemplatePolicy(domain.TemplateID("python-ds"), templatePolicy)

	ctx := context.Background()

	// Should get template policy
	policy, err := resolver.ResolvePolicy(ctx, "sandbox-1", "python-ds", ReasonUserRequest)
	require.NoError(t, err)
	require.Equal(t, "python-template", policy.ID)
	require.True(t, policy.CheckpointFirst)

	// Should get default for unknown template
	policy, err = resolver.ResolvePolicy(ctx, "sandbox-1", "unknown", ReasonUserRequest)
	require.NoError(t, err)
	require.Equal(t, "default", policy.ID)
}

func TestStaticPolicyResolver_ReasonPolicyOverridesTemplate(t *testing.T) {
	defaultPolicy := &GracePolicy{
		ID:           "default",
		DefaultGrace: 5 * time.Second,
	}
	templatePolicy := &GracePolicy{
		ID:           "template-policy",
		DefaultGrace: 15 * time.Second,
	}
	enforcementPolicy := &GracePolicy{
		ID:           "enforcement-policy",
		DefaultGrace: 2 * time.Second, // Shorter grace for enforcement
	}

	resolver := NewStaticPolicyResolver(defaultPolicy)
	resolver.SetTemplatePolicy(domain.TemplateID("python-ds"), templatePolicy)
	resolver.SetReasonPolicy(ReasonPolicyBreach, enforcementPolicy)

	ctx := context.Background()

	// Reason policy should take precedence over template
	policy, err := resolver.ResolvePolicy(ctx, "sandbox-1", "python-ds", ReasonPolicyBreach)
	require.NoError(t, err)
	require.Equal(t, "enforcement-policy", policy.ID)
	require.Equal(t, 2*time.Second, policy.DefaultGrace)

	// Different reason should still get template policy
	policy, err = resolver.ResolvePolicy(ctx, "sandbox-1", "python-ds", ReasonUserRequest)
	require.NoError(t, err)
	require.Equal(t, "template-policy", policy.ID)
}

func TestTerminationReason_String(t *testing.T) {
	tests := []struct {
		reason   TerminationReason
		expected string
	}{
		{ReasonUserRequest, "user_request"},
		{ReasonPolicyBreach, "policy_breach"},
		{ReasonResourceLimit, "resource_limit"},
		{ReasonTimeLimit, "time_limit"},
		{ReasonSystemShutdown, "system_shutdown"},
		{ReasonNetworkViolation, "network_violation"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.reason.String())
		})
	}
}
