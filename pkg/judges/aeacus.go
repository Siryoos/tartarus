package judges

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tartarus-sandbox/tartarus/pkg/cerberus"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// AeacusJudge is an audit judge that tags compliance/retention metadata and emits audit records.
type AeacusJudge struct {
	logger hermes.Logger
	sink   AuditSink
}

// NewAeacusJudge creates a new Aeacus judge with the specified audit sink.
// If sink is nil, a NoopAuditSink is used.
func NewAeacusJudge(logger hermes.Logger, sink AuditSink) *AeacusJudge {
	if sink == nil {
		sink = NewNoopAuditSink()
	}
	return &AeacusJudge{
		logger: logger,
		sink:   sink,
	}
}

// PreAdmit validates a sandbox request and adds audit metadata.
func (j *AeacusJudge) PreAdmit(ctx context.Context, req *domain.SandboxRequest) (Verdict, error) {
	// 1. Ensure Metadata map exists
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}

	// 2. Add Compliance Metadata
	auditID := uuid.New().String()
	req.Metadata["audit_id"] = auditID
	req.Metadata["audit_timestamp"] = time.Now().UTC().Format(time.RFC3339)
	req.Metadata["compliance_level"] = "standard" // Default level

	// 3. Enforce Retention Policy if missing
	if req.Retention.MaxAge == 0 {
		req.Retention.MaxAge = 1 * time.Hour // Default to 1 hour
		j.logger.Info(ctx, "Aeacus: Enforced default retention policy", map[string]any{
			"sandbox_id": req.ID,
			"audit_id":   auditID,
			"max_age":    req.Retention.MaxAge.String(),
		})
	}

	// 4. Emit Audit Record
	auditRecord := &AuditRecord{
		AuditID:         auditID,
		Timestamp:       time.Now().UTC(),
		SandboxID:       req.ID,
		TemplateID:      req.Template,
		Event:           "sandbox_request_audit",
		ComplianceLevel: req.Metadata["compliance_level"],
		RetentionPolicy: req.Retention,
		Metadata:        req.Metadata,
	}

	// Capture identity if available
	if identity, ok := cerberus.GetIdentity(ctx); ok {
		auditRecord.IdentityID = identity.ID
		auditRecord.IdentityType = string(identity.Type)
		auditRecord.TenantID = identity.TenantID
	}

	if err := j.sink.Emit(ctx, auditRecord); err != nil {
		j.logger.Error(ctx, "Failed to emit audit record", map[string]any{
			"sandbox_id": req.ID,
			"audit_id":   auditID,
			"error":      err,
		})
		// Continue even if audit sink fails - don't block request processing
	}

	return VerdictAccept, nil
}
