package judges

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// AuditRecord represents a structured audit event emitted by Aeacus.
type AuditRecord struct {
	AuditID         string                  `json:"audit_id"`
	Timestamp       time.Time               `json:"timestamp"`
	SandboxID       domain.SandboxID        `json:"sandbox_id"`
	TemplateID      domain.TemplateID       `json:"template_id"`
	Event           string                  `json:"event"`
	ComplianceLevel string                  `json:"compliance_level"`
	RetentionPolicy domain.RetentionPolicy  `json:"retention_policy"`
	Metadata        map[string]string       `json:"metadata"`
}

// AuditSink is the interface for audit record emission.
// Implementations can write to logs, databases, message queues, or external audit systems.
type AuditSink interface {
	Emit(ctx context.Context, record *AuditRecord) error
}

// LogAuditSink emits audit records via structured logging.
// This is suitable for development and basic production use.
type LogAuditSink struct {
	logger hermes.Logger
}

// NewLogAuditSink creates a new log-based audit sink.
func NewLogAuditSink(logger hermes.Logger) *LogAuditSink {
	return &LogAuditSink{
		logger: logger,
	}
}

// Emit writes the audit record as a structured log entry.
func (s *LogAuditSink) Emit(ctx context.Context, record *AuditRecord) error {
	s.logger.Info(ctx, "Aeacus: Audit Record", map[string]any{
		"event":            record.Event,
		"sandbox_id":       record.SandboxID,
		"audit_id":         record.AuditID,
		"template":         record.TemplateID,
		"compliance_level": record.ComplianceLevel,
		"retention_policy": record.RetentionPolicy,
		"timestamp":        record.Timestamp.Format(time.RFC3339),
	})
	return nil
}

// MultiAuditSink emits audit records to multiple sinks.
// If any sink fails, it continues to emit to remaining sinks and returns the first error.
type MultiAuditSink struct {
	sinks []AuditSink
}

// NewMultiAuditSink creates a new multi-sink that emits to all provided sinks.
func NewMultiAuditSink(sinks ...AuditSink) *MultiAuditSink {
	return &MultiAuditSink{
		sinks: sinks,
	}
}

// Emit writes the audit record to all configured sinks.
func (s *MultiAuditSink) Emit(ctx context.Context, record *AuditRecord) error {
	var firstErr error
	for _, sink := range s.sinks {
		if err := sink.Emit(ctx, record); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NoopAuditSink discards all audit records.
// Useful for testing or when audit is explicitly disabled.
type NoopAuditSink struct{}

// NewNoopAuditSink creates a new no-op audit sink.
func NewNoopAuditSink() *NoopAuditSink {
	return &NoopAuditSink{}
}

// Emit does nothing.
func (s *NoopAuditSink) Emit(ctx context.Context, record *AuditRecord) error {
	return nil
}

// MockAuditSink captures emitted audit records for testing.
type MockAuditSink struct {
	Records []*AuditRecord
	Err     error
}

// NewMockAuditSink creates a new mock audit sink for testing.
func NewMockAuditSink() *MockAuditSink {
	return &MockAuditSink{
		Records: make([]*AuditRecord, 0),
	}
}

// Emit captures the audit record for later verification.
func (s *MockAuditSink) Emit(ctx context.Context, record *AuditRecord) error {
	if s.Err != nil {
		return s.Err
	}
	// Deep copy the record to avoid mutation issues
	recordCopy := *record
	if record.Metadata != nil {
		recordCopy.Metadata = make(map[string]string)
		for k, v := range record.Metadata {
			recordCopy.Metadata[k] = v
		}
	}
	s.Records = append(s.Records, &recordCopy)
	return nil
}

// LastRecord returns the most recently emitted record, or nil if none.
func (s *MockAuditSink) LastRecord() *AuditRecord {
	if len(s.Records) == 0 {
		return nil
	}
	return s.Records[len(s.Records)-1]
}

// Reset clears all captured records.
func (s *MockAuditSink) Reset() {
	s.Records = make([]*AuditRecord, 0)
}

// JSONAuditSink emits audit records as JSON to a writer or logger.
// This is useful for integration with JSON-based audit aggregation systems.
type JSONAuditSink struct {
	logger hermes.Logger
}

// NewJSONAuditSink creates a new JSON-based audit sink.
func NewJSONAuditSink(logger hermes.Logger) *JSONAuditSink {
	return &JSONAuditSink{
		logger: logger,
	}
}

// Emit writes the audit record as a JSON object.
func (s *JSONAuditSink) Emit(ctx context.Context, record *AuditRecord) error {
	jsonBytes, err := json.Marshal(record)
	if err != nil {
		return err
	}
	s.logger.Info(ctx, "Aeacus: Audit Record (JSON)", map[string]any{
		"audit_record": string(jsonBytes),
	})
	return nil
}
