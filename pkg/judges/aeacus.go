package judges

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// AeacusJudge is an audit judge that tags compliance/retention metadata and emits audit records.
type AeacusJudge struct {
	logger hermes.Logger
}

// NewAeacusJudge creates a new Aeacus judge.
func NewAeacusJudge(logger hermes.Logger) *AeacusJudge {
	return &AeacusJudge{
		logger: logger,
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
	j.logger.Info(ctx, "Aeacus: Audit Record", map[string]any{
		"event":            "sandbox_request_audit",
		"sandbox_id":       req.ID,
		"audit_id":         auditID,
		"template":         req.Template,
		"compliance_level": req.Metadata["compliance_level"],
		"retention_policy": req.Retention,
	})

	return VerdictAccept, nil
}
