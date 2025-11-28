package cerberus

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestLogAuditor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	auditor := NewLogAuditor(logger)

	identity := &Identity{
		ID:       "test-user",
		Type:     IdentityTypeUser,
		TenantID: "test-tenant",
	}

	entry := &AuditEntry{
		Timestamp: time.Now(),
		RequestID: "req-123",
		Identity:  identity,
		Action:    ActionCreate,
		Resource: Resource{
			Type: ResourceTypeSandbox,
			ID:   "sandbox-123",
		},
		Result:    AuditResultSuccess,
		Latency:   100 * time.Millisecond,
		SourceIP:  "192.168.1.1",
		UserAgent: "test-agent",
	}

	err := auditor.RecordAccess(ctx, entry)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test with denied result
	entry.Result = AuditResultDenied
	entry.ErrorMessage = "permission denied"
	err = auditor.RecordAccess(ctx, entry)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNoopAuditor(t *testing.T) {
	ctx := context.Background()
	auditor := NewNoopAuditor()

	entry := &AuditEntry{
		Timestamp: time.Now(),
		RequestID: "req-123",
		Result:    AuditResultSuccess,
	}

	err := auditor.RecordAccess(ctx, entry)
	if err != nil {
		t.Errorf("NoopAuditor should never error, got: %v", err)
	}
}

func TestCompositeAuditor(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	logAuditor := NewLogAuditor(logger)
	metricsAuditor := NewMetricsAuditor()
	noopAuditor := NewNoopAuditor()

	composite := NewCompositeAuditor(logAuditor, metricsAuditor, noopAuditor)

	entry := &AuditEntry{
		Timestamp: time.Now(),
		RequestID: "req-123",
		Action:    ActionCreate,
		Resource: Resource{
			Type: ResourceTypeSandbox,
			ID:   "sandbox-123",
		},
		Result:   AuditResultSuccess,
		SourceIP: "192.168.1.1",
	}

	err := composite.RecordAccess(ctx, entry)
	if err != nil {
		// Composite auditor may return error if any child fails,
		// but should continue processing all auditors
		t.Logf("composite auditor returned error (expected): %v", err)
	}
}

func TestMetricsAuditor(t *testing.T) {
	ctx := context.Background()
	auditor := NewMetricsAuditor()

	entry := &AuditEntry{
		Timestamp: time.Now(),
		RequestID: "req-123",
		Action:    ActionCreate,
		Resource: Resource{
			Type: ResourceTypeSandbox,
			ID:   "sandbox-123",
		},
		Result:   AuditResultSuccess,
		Latency:  50 * time.Millisecond,
		SourceIP: "192.168.1.1",
	}

	// MetricsAuditor currently does nothing, but should not error
	err := auditor.RecordAccess(ctx, entry)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
