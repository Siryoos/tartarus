package thanatos

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

const (
	// DefaultGracePeriod is the fallback grace window if no policy matches.
	DefaultGracePeriod = 5 * time.Second
	// MaxGracePeriod is the hard cap on any grace window.
	MaxGracePeriod = 5 * time.Minute
)

// GracePolicy defines termination behavior for a sandbox.
type GracePolicy struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	DefaultGrace    time.Duration `json:"default_grace"`    // Default grace period
	MaxGrace        time.Duration `json:"max_grace"`        // Hard cap on grace
	CheckpointFirst bool          `json:"checkpoint_first"` // Require checkpoint before kill
	ExportLogs      bool          `json:"export_logs"`      // Export logs before termination
	ExportArtifacts bool          `json:"export_artifacts"` // Export artifacts before termination
}

// Clone creates a deep copy of the policy.
func (p *GracePolicy) Clone() *GracePolicy {
	if p == nil {
		return nil
	}
	return &GracePolicy{
		ID:              p.ID,
		Name:            p.Name,
		DefaultGrace:    p.DefaultGrace,
		MaxGrace:        p.MaxGrace,
		CheckpointFirst: p.CheckpointFirst,
		ExportLogs:      p.ExportLogs,
		ExportArtifacts: p.ExportArtifacts,
	}
}

// EffectiveGrace returns the grace period to use, clamped to MaxGrace.
func (p *GracePolicy) EffectiveGrace(requested time.Duration) time.Duration {
	if requested <= 0 {
		requested = p.DefaultGrace
	}
	if requested <= 0 {
		requested = DefaultGracePeriod
	}
	if p.MaxGrace > 0 && requested > p.MaxGrace {
		return p.MaxGrace
	}
	if requested > MaxGracePeriod {
		return MaxGracePeriod
	}
	return requested
}

// PolicyResolver resolves the applicable grace policy for a sandbox.
type PolicyResolver interface {
	// ResolvePolicy finds the best matching policy for termination.
	ResolvePolicy(ctx context.Context, sandboxID domain.SandboxID, templateID domain.TemplateID, reason TerminationReason) (*GracePolicy, error)
}

// StaticPolicyResolver uses static mappings to resolve policies.
// Resolution order: ReasonPolicy > TemplatePolicy > Default
type StaticPolicyResolver struct {
	Default        *GracePolicy
	TemplatePolicy map[domain.TemplateID]*GracePolicy
	ReasonPolicy   map[TerminationReason]*GracePolicy
}

// NewStaticPolicyResolver creates a new resolver with the given default policy.
func NewStaticPolicyResolver(defaultPolicy *GracePolicy) *StaticPolicyResolver {
	if defaultPolicy == nil {
		defaultPolicy = &GracePolicy{
			ID:           "default",
			Name:         "Default Policy",
			DefaultGrace: DefaultGracePeriod,
			MaxGrace:     MaxGracePeriod,
		}
	}
	return &StaticPolicyResolver{
		Default:        defaultPolicy,
		TemplatePolicy: make(map[domain.TemplateID]*GracePolicy),
		ReasonPolicy:   make(map[TerminationReason]*GracePolicy),
	}
}

// SetTemplatePolicy associates a policy with a template.
func (r *StaticPolicyResolver) SetTemplatePolicy(templateID domain.TemplateID, policy *GracePolicy) {
	r.TemplatePolicy[templateID] = policy
}

// SetReasonPolicy associates a policy with a termination reason.
func (r *StaticPolicyResolver) SetReasonPolicy(reason TerminationReason, policy *GracePolicy) {
	r.ReasonPolicy[reason] = policy
}

// ResolvePolicy finds the best matching policy.
// Priority: ReasonPolicy > TemplatePolicy > Default
func (r *StaticPolicyResolver) ResolvePolicy(_ context.Context, _ domain.SandboxID, templateID domain.TemplateID, reason TerminationReason) (*GracePolicy, error) {
	// Check reason-specific policy first (highest priority)
	if policy, ok := r.ReasonPolicy[reason]; ok && policy != nil {
		return policy.Clone(), nil
	}

	// Check template-specific policy
	if policy, ok := r.TemplatePolicy[templateID]; ok && policy != nil {
		return policy.Clone(), nil
	}

	// Fall back to default
	return r.Default.Clone(), nil
}

// TerminationReason categorizes why termination was requested.
type TerminationReason string

const (
	// ReasonUserRequest indicates user-initiated termination.
	ReasonUserRequest TerminationReason = "user_request"
	// ReasonPolicyBreach indicates termination due to policy violation.
	ReasonPolicyBreach TerminationReason = "policy_breach"
	// ReasonResourceLimit indicates termination due to resource exhaustion.
	ReasonResourceLimit TerminationReason = "resource_limit"
	// ReasonTimeLimit indicates termination due to time quota.
	ReasonTimeLimit TerminationReason = "time_limit"
	// ReasonSystemShutdown indicates system-wide shutdown.
	ReasonSystemShutdown TerminationReason = "system_shutdown"
	// ReasonNetworkViolation indicates network policy violation.
	ReasonNetworkViolation TerminationReason = "network_violation"
)

// String returns the string representation of the reason.
func (r TerminationReason) String() string {
	return string(r)
}
