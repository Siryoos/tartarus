package cerberus

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes/audit"
)

// Auditor records access attempts for compliance and security monitoring.
type Auditor interface {
	RecordAccess(ctx context.Context, entry *AuditEntry) error
}

// HermesAuditor adapts Cerberus audit entries to Hermes audit events.
type HermesAuditor struct {
	auditor audit.Auditor
}

// NewHermesAuditor creates a new auditor that delegates to Hermes.
func NewHermesAuditor(a audit.Auditor) *HermesAuditor {
	return &HermesAuditor{
		auditor: a,
	}
}

// RecordAccess converts the entry and records it via Hermes.
func (a *HermesAuditor) RecordAccess(ctx context.Context, entry *AuditEntry) error {
	event := &audit.Event{
		ID:        entry.RequestID, // Use RequestID as Event ID for correlation, or let Hermes generate one?
		Timestamp: entry.Timestamp,
		Action:    audit.Action(entry.Action),
		Result:    audit.Result(entry.Result),
		Resource: audit.Resource{
			Type: string(entry.Resource.Type),
			ID:   entry.Resource.ID,
			Name: entry.Resource.Namespace, // Mapping Namespace to Name for now
		},
		SourceIP:     entry.SourceIP,
		UserAgent:    entry.UserAgent,
		RequestID:    entry.RequestID,
		Latency:      entry.Latency,
		ErrorMessage: entry.ErrorMessage,
	}

	if entry.Identity != nil {
		event.Identity = &audit.Identity{
			ID:       entry.Identity.ID,
			Type:     string(entry.Identity.Type),
			TenantID: entry.Identity.TenantID,
		}
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	return a.auditor.Record(ctx, event)
}

// MetricsAuditor emits metrics for access attempts.
// This integrates with the Hermes metrics system.
type MetricsAuditor struct {
	metrics hermes.Metrics
}

// NewMetricsAuditor creates an auditor that emits metrics.
func NewMetricsAuditor(metrics hermes.Metrics) *MetricsAuditor {
	return &MetricsAuditor{
		metrics: metrics,
	}
}

// RecordAccess emits metrics for the access attempt.
func (m *MetricsAuditor) RecordAccess(ctx context.Context, entry *AuditEntry) error {
	m.metrics.IncCounter("cerberus_access_total", 1,
		hermes.Label{Key: "result", Value: string(entry.Result)},
		hermes.Label{Key: "action", Value: string(entry.Action)},
		hermes.Label{Key: "resource_type", Value: string(entry.Resource.Type)},
	)

	m.metrics.ObserveHistogram("cerberus_access_latency_seconds", entry.Latency.Seconds(),
		hermes.Label{Key: "action", Value: string(entry.Action)},
		hermes.Label{Key: "resource_type", Value: string(entry.Resource.Type)},
	)

	if entry.Result != AuditResultSuccess {
		reason := "unknown"
		if entry.ErrorMessage != "" {
			// Simple heuristic for reason, in real world we might want structured error codes
			if entry.Result == AuditResultDenied {
				reason = "denied"
			} else {
				reason = "error"
			}
		}
		m.metrics.IncCounter("cerberus_auth_failures_total", 1,
			hermes.Label{Key: "reason", Value: reason},
		)
	}

	return nil
}

// CompositeAuditor combines multiple auditors.
type CompositeAuditor struct {
	auditors []Auditor
}

// NewCompositeAuditor creates an auditor that delegates to multiple auditors.
func NewCompositeAuditor(auditors ...Auditor) *CompositeAuditor {
	return &CompositeAuditor{
		auditors: auditors,
	}
}

// RecordAccess calls all configured auditors.
// Errors from individual auditors are logged but don't fail the operation.
func (c *CompositeAuditor) RecordAccess(ctx context.Context, entry *AuditEntry) error {
	var firstErr error

	for _, auditor := range c.auditors {
		if err := auditor.RecordAccess(ctx, entry); err != nil {
			if firstErr == nil {
				firstErr = NewAuditError("composite auditor failed", err)
			}
			// Continue to other auditors even if one fails
		}
	}

	return firstErr
}

// NoopAuditor does nothing. Useful for testing or when audit is disabled.
type NoopAuditor struct{}

// NewNoopAuditor creates an auditor that does nothing.
func NewNoopAuditor() *NoopAuditor {
	return &NoopAuditor{}
}

// RecordAccess does nothing.
func (n *NoopAuditor) RecordAccess(ctx context.Context, entry *AuditEntry) error {
	return nil
}
