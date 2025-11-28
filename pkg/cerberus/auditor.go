package cerberus

import (
	"context"
	"log/slog"
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
	// In a real implementation, this would use pkg/hermes
	// For now, we'll keep it simple
}

// NewMetricsAuditor creates an auditor that emits metrics.
func NewMetricsAuditor() *MetricsAuditor {
	return &MetricsAuditor{}
}

// RecordAccess emits metrics for the access attempt.
func (m *MetricsAuditor) RecordAccess(ctx context.Context, entry *AuditEntry) error {
	// TODO: Integrate with pkg/hermes to emit:
	// - Counter: cerberus_access_total{result, action, resource_type}
	// - Histogram: cerberus_access_latency{action, resource_type}
	// - Counter: cerberus_auth_failures_total{reason}
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
