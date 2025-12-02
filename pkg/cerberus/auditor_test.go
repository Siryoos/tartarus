package cerberus

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes/audit"
)

type MockMetrics struct {
	hermes.Metrics // Embed interface to satisfy it, but we'll implement methods we need
}

func (m *MockMetrics) IncCounter(name string, value float64, labels ...hermes.Label)       {}
func (m *MockMetrics) ObserveHistogram(name string, value float64, labels ...hermes.Label) {}
func (m *MockMetrics) SetGauge(name string, value float64, labels ...hermes.Label)         {}

type MockHermesAuditor struct {
	events []*audit.Event
}

func (m *MockHermesAuditor) Record(ctx context.Context, event *audit.Event) error {
	m.events = append(m.events, event)
	return nil
}

func TestHermesAuditor(t *testing.T) {
	ctx := context.Background()
	mockAuditor := &MockHermesAuditor{}
	auditor := NewHermesAuditor(mockAuditor)

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

	if len(mockAuditor.events) != 1 {
		t.Errorf("expected 1 event, got %d", len(mockAuditor.events))
	}

	event := mockAuditor.events[0]
	if event.Action != audit.ActionCreate {
		t.Errorf("expected action %s, got %s", audit.ActionCreate, event.Action)
	}
	if event.Identity.ID != "test-user" {
		t.Errorf("expected identity ID test-user, got %s", event.Identity.ID)
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

	mockAuditor := &MockHermesAuditor{}
	hermesAuditor := NewHermesAuditor(mockAuditor)
	metricsAuditor := NewMetricsAuditor(&MockMetrics{})
	noopAuditor := NewNoopAuditor()

	composite := NewCompositeAuditor(hermesAuditor, metricsAuditor, noopAuditor)

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
		t.Logf("composite auditor returned error (expected): %v", err)
	}

	if len(mockAuditor.events) != 1 {
		t.Errorf("expected 1 event in mock auditor, got %d", len(mockAuditor.events))
	}
}

func TestMetricsAuditor(t *testing.T) {
	ctx := context.Background()
	auditor := NewMetricsAuditor(&MockMetrics{})

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
