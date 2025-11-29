package judges

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

func TestAeacusJudge_PreAdmit(t *testing.T) {
	logger := hermes.NewNoopLogger()
	ctx := context.Background()

	t.Run("AddsAuditMetadata", func(t *testing.T) {
		mockSink := NewMockAuditSink()
		judge := NewAeacusJudge(logger, mockSink)

		req := &domain.SandboxRequest{
			ID:       "test-sandbox",
			Template: "test-template",
		}

		verdict, err := judge.PreAdmit(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if verdict != VerdictAccept {
			t.Errorf("expected VerdictAccept, got %v", verdict)
		}

		if req.Metadata["audit_id"] == "" {
			t.Error("expected audit_id to be set")
		}
		if req.Metadata["audit_timestamp"] == "" {
			t.Error("expected audit_timestamp to be set")
		}
		if req.Metadata["compliance_level"] != "standard" {
			t.Errorf("expected compliance_level 'standard', got '%s'", req.Metadata["compliance_level"])
		}

		// Verify audit record was emitted
		if len(mockSink.Records) != 1 {
			t.Fatalf("expected 1 audit record, got %d", len(mockSink.Records))
		}

		auditRecord := mockSink.LastRecord()
		if auditRecord.AuditID != req.Metadata["audit_id"] {
			t.Errorf("audit record AuditID mismatch")
		}
		if auditRecord.SandboxID != req.ID {
			t.Errorf("expected SandboxID %s, got %s", req.ID, auditRecord.SandboxID)
		}
		if auditRecord.TemplateID != req.Template {
			t.Errorf("expected TemplateID %s, got %s", req.Template, auditRecord.TemplateID)
		}
		if auditRecord.Event != "sandbox_request_audit" {
			t.Errorf("expected Event 'sandbox_request_audit', got '%s'", auditRecord.Event)
		}
		if auditRecord.ComplianceLevel != "standard" {
			t.Errorf("expected ComplianceLevel 'standard', got '%s'", auditRecord.ComplianceLevel)
		}
	})

	t.Run("EnforcesDefaultRetention", func(t *testing.T) {
		mockSink := NewMockAuditSink()
		judge := NewAeacusJudge(logger, mockSink)

		req := &domain.SandboxRequest{
			ID:       "test-sandbox-retention",
			Template: "test-template",
		}

		_, err := judge.PreAdmit(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if req.Retention.MaxAge != 1*time.Hour {
			t.Errorf("expected default retention 1h, got %v", req.Retention.MaxAge)
		}
	})

	t.Run("RespectsExistingRetention", func(t *testing.T) {
		mockSink := NewMockAuditSink()
		judge := NewAeacusJudge(logger, mockSink)

		req := &domain.SandboxRequest{
			ID:       "test-sandbox-existing-retention",
			Template: "test-template",
			Retention: domain.RetentionPolicy{
				MaxAge: 24 * time.Hour,
			},
		}

		_, err := judge.PreAdmit(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if req.Retention.MaxAge != 24*time.Hour {
			t.Errorf("expected retention 24h, got %v", req.Retention.MaxAge)
		}
	})
}
