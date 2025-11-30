package typhon

import (
	"context"
	"fmt"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// HardenedQuarantineManager wraps a basic QuarantineManager with security hardening
type HardenedQuarantineManager struct {
	base       QuarantineManager
	policy     *QuarantinePolicy
	classifier Classifier
	logger     hermes.Logger
	metrics    hermes.Metrics
}

// NewHardenedQuarantineManager creates a hardened quarantine manager
func NewHardenedQuarantineManager(
	base QuarantineManager,
	policy *QuarantinePolicy,
	classifier Classifier,
	logger hermes.Logger,
	metrics hermes.Metrics,
) *HardenedQuarantineManager {
	// Set default network mode if not specified
	if policy.DefaultNetworkMode == "" {
		policy.DefaultNetworkMode = NetworkModeNone
	}

	// Set default seccomp profile if not specified
	if policy.Isolation.SeccompProfile == "" {
		policy.Isolation.SeccompProfile = SeccompQuarantine
	}

	// Propagate default network mode to isolation config
	if policy.Isolation.NetworkMode == "" {
		policy.Isolation.NetworkMode = policy.DefaultNetworkMode
	}

	// Initialize storage config if not set
	if policy.Isolation.StorageConfig == nil {
		policy.Isolation.StorageConfig = &QuarantineStorageConfig{
			IsolatedDir:          "/var/lib/tartarus/quarantine",
			UseSnapshotIsolation: true,
			SnapshotPrefix:       "quarantine:",
		}
	}

	return &HardenedQuarantineManager{
		base:       base,
		policy:     policy,
		classifier: classifier,
		logger:     logger,
		metrics:    metrics,
	}
}

// Quarantine moves a sandbox to quarantine with enhanced isolation
// Quarantine moves a sandbox to quarantine with enhanced isolation
func (h *HardenedQuarantineManager) Quarantine(ctx context.Context, req *QuarantineRequest) (*QuarantineRecord, error) {
	h.logger.Info(ctx, "quarantine_request", map[string]any{
		"sandbox_id": req.SandboxID,
		"reason":     req.Reason,
		"auto":       req.AutoQuarantine,
	})

	// Enforce default isolation settings
	if err := h.enforceIsolation(req); err != nil {
		h.metrics.IncCounter("typhon.quarantine.isolation_enforcement_failed", 1)
		return nil, fmt.Errorf("isolation enforcement failed: %w", err)
	}

	// Call base manager
	record, err := h.base.Quarantine(ctx, req)
	if err != nil {
		h.metrics.IncCounter("typhon.quarantine.failed", 1)
		return nil, err
	}

	// Log security event
	h.logSecurityEvent(ctx, "quarantine", req.SandboxID, map[string]interface{}{
		"reason":          req.Reason,
		"auto_quarantine": req.AutoQuarantine,
		"network_mode":    h.policy.DefaultNetworkMode,
		"seccomp":         h.policy.Isolation.SeccompProfile,
		"storage_dir":     h.policy.Isolation.StorageConfig.IsolatedDir,
	})

	h.metrics.IncCounter("typhon.quarantine.success", 1)
	if req.AutoQuarantine {
		h.metrics.IncCounter("typhon.quarantine.auto", 1)
	} else {
		h.metrics.IncCounter("typhon.quarantine.manual", 1)
	}

	return record, nil
}

// Release removes a sandbox from quarantine with override handling
func (h *HardenedQuarantineManager) Release(ctx context.Context, sandboxID string, approval *ReleaseApproval) error {
	h.logger.Info(ctx, "quarantine_release", map[string]any{
		"sandbox_id":  sandboxID,
		"approved_by": approval.ApprovedBy,
	})

	// Apply overrides if present
	if approval.NetworkOverride != nil {
		h.logSecurityEvent(ctx, "network_override", sandboxID, map[string]interface{}{
			"network_mode":   approval.NetworkOverride.NetworkMode,
			"allowed_egress": approval.NetworkOverride.AllowedEgress,
			"justification":  approval.NetworkOverride.Justification,
			"approved_by":    approval.ApprovedBy,
		})
		h.metrics.IncCounter("typhon.quarantine.network_override", 1)
	}

	if approval.SecurityOverride != nil {
		h.logSecurityEvent(ctx, "security_override", sandboxID, map[string]interface{}{
			"seccomp_profile": approval.SecurityOverride.SeccompProfile,
			"justification":   approval.SecurityOverride.Justification,
			"approved_by":     approval.ApprovedBy,
		})
		h.metrics.IncCounter("typhon.quarantine.security_override", 1)
	}

	// Call base manager
	if err := h.base.Release(ctx, sandboxID, approval); err != nil {
		h.metrics.IncCounter("typhon.quarantine.release_failed", 1)
		return err
	}

	h.logSecurityEvent(ctx, "release", sandboxID, map[string]interface{}{
		"approved_by": approval.ApprovedBy,
		"reason":      approval.Reason,
	})

	h.metrics.IncCounter("typhon.quarantine.release_success", 1)
	return nil
}

// Examine analyzes a quarantined sandbox
func (h *HardenedQuarantineManager) Examine(ctx context.Context, sandboxID string) (*ExaminationReport, error) {
	return h.base.Examine(ctx, sandboxID)
}

// ListQuarantined returns all sandboxes in quarantine
func (h *HardenedQuarantineManager) ListQuarantined(ctx context.Context, filter *QuarantineFilter) ([]*QuarantineRecord, error) {
	return h.base.ListQuarantined(ctx, filter)
}

// SetPolicy configures quarantine policies
func (h *HardenedQuarantineManager) SetPolicy(ctx context.Context, policy *QuarantinePolicy) error {
	// Enforce security defaults
	if policy.DefaultNetworkMode == "" {
		policy.DefaultNetworkMode = NetworkModeNone
	}
	if policy.Isolation.SeccompProfile == "" {
		policy.Isolation.SeccompProfile = SeccompQuarantine
	}

	h.policy = policy
	return h.base.SetPolicy(ctx, policy)
}

// Classify evaluates if a sandbox should be quarantined
func (h *HardenedQuarantineManager) Classify(ctx context.Context, sandbox *domain.SandboxRequest) (bool, QuarantineReason, []Evidence) {
	shouldQuarantine, reason, evidence := h.classifier.ShouldQuarantine(ctx, sandbox)

	if shouldQuarantine {
		h.logger.Info(ctx, "auto_classification_triggered", map[string]any{
			"sandbox_id": sandbox.ID,
			"reason":     reason,
		})
		h.metrics.IncCounter("typhon.classification.triggered", 1)
		h.metrics.IncCounter(fmt.Sprintf("typhon.classification.reason.%s", reason), 1)
	} else {
		h.metrics.IncCounter("typhon.classification.passed", 1)
	}

	return shouldQuarantine, reason, evidence
}

// GetIsolationConfig returns the current isolation configuration
func (h *HardenedQuarantineManager) GetIsolationConfig() QuarantineIsolation {
	return h.policy.Isolation
}

// enforceIsolation ensures default isolation settings are applied
func (h *HardenedQuarantineManager) enforceIsolation(req *QuarantineRequest) error {
	// Validate that evidence is provided for auto-quarantine
	if req.AutoQuarantine && len(req.Evidence) == 0 {
		return fmt.Errorf("auto-quarantine requires evidence")
	}

	// Note: Actual enforcement of network mode, seccomp, and storage
	// happens at the runtime level (Firecracker/Nyx integration)
	// This manager sets the policy that those systems will enforce

	return nil
}

// logSecurityEvent logs a security-relevant event for audit trail
func (h *HardenedQuarantineManager) logSecurityEvent(ctx context.Context, event string, sandboxID string, details map[string]interface{}) {
	fields := map[string]any{
		"event_type": event,
		"sandbox_id": sandboxID,
		"timestamp":  time.Now().UTC(),
	}
	for k, v := range details {
		fields[k] = v
	}
	h.logger.Info(ctx, "security_event", fields)
}
