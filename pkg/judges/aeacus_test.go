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
	judge := NewAeacusJudge(logger)
	ctx := context.Background()

	t.Run("AddsAuditMetadata", func(t *testing.T) {
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
	})

	t.Run("EnforcesDefaultRetention", func(t *testing.T) {
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
