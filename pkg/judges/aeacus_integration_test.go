package judges

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// TestAeacusJudge_Integration tests the full audit flow from request submission to audit record emission.
func TestAeacusJudge_Integration(t *testing.T) {
	ctx := context.Background()
	logger := hermes.NewNoopLogger()

	t.Run("SingleRequestAuditFlow", func(t *testing.T) {
		// Setup
		mockSink := NewMockAuditSink()
		aeacusJudge := NewAeacusJudge(logger, mockSink)

		// Create sandbox request
		reqID := domain.SandboxID(uuid.New().String())
		req := &domain.SandboxRequest{
			ID:       reqID,
			Template: "test-template",
			Resources: domain.ResourceSpec{
				CPU: 1000,
				Mem: 512,
			},
		}

		// Execute PreAdmit
		verdict, err := aeacusJudge.PreAdmit(ctx, req)

		// Verify verdict
		require.NoError(t, err)
		assert.Equal(t, VerdictAccept, verdict)

		// Verify metadata was added to request
		assert.NotEmpty(t, req.Metadata["audit_id"])
		assert.NotEmpty(t, req.Metadata["audit_timestamp"])
		assert.Equal(t, "standard", req.Metadata["compliance_level"])

		// Verify retention policy was enforced
		assert.Equal(t, 1*time.Hour, req.Retention.MaxAge)

		// Verify audit record was emitted
		require.Len(t, mockSink.Records, 1)
		record := mockSink.LastRecord()

		assert.Equal(t, req.Metadata["audit_id"], record.AuditID)
		assert.Equal(t, reqID, record.SandboxID)
		assert.Equal(t, req.Template, record.TemplateID)
		assert.Equal(t, "sandbox_request_audit", record.Event)
		assert.Equal(t, "standard", record.ComplianceLevel)
		assert.Equal(t, 1*time.Hour, record.RetentionPolicy.MaxAge)
		assert.NotNil(t, record.Metadata)
		assert.Equal(t, req.Metadata["audit_id"], record.Metadata["audit_id"])
	})

	t.Run("MultipleRequestsAuditFlow", func(t *testing.T) {
		// Setup
		mockSink := NewMockAuditSink()
		aeacusJudge := NewAeacusJudge(logger, mockSink)

		// Submit 3 requests
		for i := 0; i < 3; i++ {
			req := &domain.SandboxRequest{
				ID:       domain.SandboxID(uuid.New().String()),
				Template: "test-template",
			}

			verdict, err := aeacusJudge.PreAdmit(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, VerdictAccept, verdict)
		}

		// Verify 3 audit records were emitted
		assert.Len(t, mockSink.Records, 3)

		// Verify each has unique audit ID
		auditIDs := make(map[string]bool)
		for _, record := range mockSink.Records {
			assert.NotEmpty(t, record.AuditID)
			auditIDs[record.AuditID] = true
		}
		assert.Len(t, auditIDs, 3, "All audit IDs should be unique")
	})

	t.Run("AuditSinkFailureDoesNotBlockRequest", func(t *testing.T) {
		// Setup sink that always fails
		mockSink := NewMockAuditSink()
		mockSink.Err = assert.AnError
		aeacusJudge := NewAeacusJudge(logger, mockSink)

		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
		}

		// Execute PreAdmit
		verdict, err := aeacusJudge.PreAdmit(ctx, req)

		// Request should still be accepted even if audit sink fails
		require.NoError(t, err)
		assert.Equal(t, VerdictAccept, verdict)

		// Metadata should still be added
		assert.NotEmpty(t, req.Metadata["audit_id"])
	})

	t.Run("WithCustomRetentionPolicy", func(t *testing.T) {
		mockSink := NewMockAuditSink()
		aeacusJudge := NewAeacusJudge(logger, mockSink)

		customRetention := domain.RetentionPolicy{
			MaxAge:      48 * time.Hour,
			KeepOutputs: true,
		}

		req := &domain.SandboxRequest{
			ID:        domain.SandboxID(uuid.New().String()),
			Template:  "test-template",
			Retention: customRetention,
		}

		verdict, err := aeacusJudge.PreAdmit(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, VerdictAccept, verdict)

		// Verify retention wasn't overwritten
		assert.Equal(t, customRetention.MaxAge, req.Retention.MaxAge)
		assert.Equal(t, customRetention.KeepOutputs, req.Retention.KeepOutputs)

		// Verify audit record has correct retention
		record := mockSink.LastRecord()
		assert.Equal(t, customRetention.MaxAge, record.RetentionPolicy.MaxAge)
		assert.Equal(t, customRetention.KeepOutputs, record.RetentionPolicy.KeepOutputs)
	})

	t.Run("WithMetadata", func(t *testing.T) {
		mockSink := NewMockAuditSink()
		aeacusJudge := NewAeacusJudge(logger, mockSink)

		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
			Metadata: map[string]string{
				"user":        "alice@example.com",
				"environment": "production",
			},
		}

		verdict, err := aeacusJudge.PreAdmit(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, VerdictAccept, verdict)

		// Verify audit record includes original metadata plus audit fields
		record := mockSink.LastRecord()
		assert.Equal(t, "alice@example.com", record.Metadata["user"])
		assert.Equal(t, "production", record.Metadata["environment"])
		assert.NotEmpty(t, record.Metadata["audit_id"])
		assert.NotEmpty(t, record.Metadata["audit_timestamp"])
		assert.Equal(t, "standard", record.Metadata["compliance_level"])
	})
}

// TestAeacusJudge_ChainIntegration tests Aeacus as part of a judge chain.
func TestAeacusJudge_ChainIntegration(t *testing.T) {
	ctx := context.Background()
	logger := hermes.NewNoopLogger()

	t.Run("FirstInChain", func(t *testing.T) {
		mockSink := NewMockAuditSink()
		aeacusJudge := NewAeacusJudge(logger, mockSink)

		// Create a simple chain with just Aeacus
		chain := &Chain{
			Pre: []PreJudge{aeacusJudge},
		}

		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
		}

		verdict, err := chain.RunPre(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, VerdictAccept, verdict)

		// Verify audit record was emitted
		assert.Len(t, mockSink.Records, 1)
		assert.NotEmpty(t, req.Metadata["audit_id"])
	})

	t.Run("NilSinkUsesNoop", func(t *testing.T) {
		// Verify that passing nil sink creates a NoopAuditSink
		aeacusJudge := NewAeacusJudge(logger, nil)

		req := &domain.SandboxRequest{
			ID:       domain.SandboxID(uuid.New().String()),
			Template: "test-template",
		}

		// Should not panic or error
		verdict, err := aeacusJudge.PreAdmit(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, VerdictAccept, verdict)
		assert.NotEmpty(t, req.Metadata["audit_id"])
	})
}
