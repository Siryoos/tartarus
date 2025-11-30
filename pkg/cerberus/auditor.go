package cerberus

import (
	"context"
	"log/slog"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// Auditor records access attempts for compliance and security monitoring.
type Auditor interface {
	RecordAccess(ctx context.Context, entry *AuditEntry) error
}

// LogAuditor writes audit entries to structured logs.
type LogAuditor struct {
	logger *slog.Logger
}

// NewLogAuditor creates an auditor that writes to logs.
func NewLogAuditor(logger *slog.Logger) *LogAuditor {
	return &LogAuditor{
		logger: logger,
	}
}

// RecordAccess logs the audit entry.
func (a *LogAuditor) RecordAccess(ctx context.Context, entry *AuditEntry) error {
	attrs := []slog.Attr{
		slog.String("request_id", entry.RequestID),
		slog.String("action", string(entry.Action)),
		slog.String("resource_type", string(entry.Resource.Type)),
		slog.String("resource_id", entry.Resource.ID),
		slog.String("result", string(entry.Result)),
		slog.Duration("latency", entry.Latency),
		slog.String("source_ip", entry.SourceIP),
		slog.String("user_agent", entry.UserAgent),
	}

	if entry.Identity != nil {
		attrs = append(attrs,
			slog.String("identity_id", entry.Identity.ID),
			slog.String("identity_type", string(entry.Identity.Type)),
			slog.String("tenant_id", entry.Identity.TenantID),
		)
	}

	if entry.ErrorMessage != "" {
		attrs = append(attrs, slog.String("error", entry.ErrorMessage))
	}

	level := slog.LevelInfo
	message := "access granted"

	switch entry.Result {
	case AuditResultDenied:
		level = slog.LevelWarn
		message = "access denied"
	case AuditResultError:
		level = slog.LevelError
		message = "access error"
	}

	a.logger.LogAttrs(ctx, level, message, attrs...)
	return nil
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
