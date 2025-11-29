package judges

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

func TestLogAuditSink(t *testing.T) {
	logger := hermes.NewNoopLogger()
	sink := NewLogAuditSink(logger)
	ctx := context.Background()

	record := &AuditRecord{
		AuditID:         "test-audit-123",
		Timestamp:       time.Now(),
		SandboxID:       "sandbox-123",
		TemplateID:      "test-template",
		Event:           "sandbox_request_audit",
		ComplianceLevel: "standard",
		RetentionPolicy: domain.RetentionPolicy{MaxAge: time.Hour},
		Metadata:        map[string]string{"key": "value"},
	}

	err := sink.Emit(ctx, record)
	if err != nil {
		t.Errorf("LogAuditSink.Emit() unexpected error: %v", err)
	}
}

func TestMultiAuditSink(t *testing.T) {
	ctx := context.Background()

	t.Run("EmitsToAllSinks", func(t *testing.T) {
		mock1 := NewMockAuditSink()
		mock2 := NewMockAuditSink()
		multi := NewMultiAuditSink(mock1, mock2)

		record := &AuditRecord{
			AuditID:    "test-audit",
			SandboxID:  "sandbox-123",
			TemplateID: "template-1",
			Event:      "test",
		}

		err := multi.Emit(ctx, record)
		if err != nil {
			t.Errorf("MultiAuditSink.Emit() unexpected error: %v", err)
		}

		if len(mock1.Records) != 1 {
			t.Errorf("Expected mock1 to have 1 record, got %d", len(mock1.Records))
		}
		if len(mock2.Records) != 1 {
			t.Errorf("Expected mock2 to have 1 record, got %d", len(mock2.Records))
		}
	})

	t.Run("ReturnsFirstError", func(t *testing.T) {
		mock1 := NewMockAuditSink()
		mock1.Err = errors.New("sink1 error")
		mock2 := NewMockAuditSink()
		multi := NewMultiAuditSink(mock1, mock2)

		record := &AuditRecord{AuditID: "test"}

		err := multi.Emit(ctx, record)
		if err == nil {
			t.Error("Expected error from MultiAuditSink.Emit()")
		}
		if err.Error() != "sink1 error" {
			t.Errorf("Expected 'sink1 error', got %v", err)
		}

		// Should still emit to mock2 despite mock1 error
		if len(mock2.Records) != 1 {
			t.Errorf("Expected mock2 to have 1 record despite error, got %d", len(mock2.Records))
		}
	})
}

func TestNoopAuditSink(t *testing.T) {
	sink := NewNoopAuditSink()
	ctx := context.Background()

	record := &AuditRecord{AuditID: "test"}
	err := sink.Emit(ctx, record)
	if err != nil {
		t.Errorf("NoopAuditSink.Emit() should never error, got: %v", err)
	}
}

func TestMockAuditSink(t *testing.T) {
	sink := NewMockAuditSink()
	ctx := context.Background()

	t.Run("CapturesRecords", func(t *testing.T) {
		record1 := &AuditRecord{
			AuditID:    "audit-1",
			SandboxID:  "sandbox-1",
			TemplateID: "template-1",
		}
		record2 := &AuditRecord{
			AuditID:    "audit-2",
			SandboxID:  "sandbox-2",
			TemplateID: "template-2",
		}

		err := sink.Emit(ctx, record1)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		err = sink.Emit(ctx, record2)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if len(sink.Records) != 2 {
			t.Errorf("Expected 2 records, got %d", len(sink.Records))
		}

		last := sink.LastRecord()
		if last.AuditID != "audit-2" {
			t.Errorf("Expected LastRecord to be audit-2, got %s", last.AuditID)
		}
	})

	t.Run("Reset", func(t *testing.T) {
		sink.Reset()
		if len(sink.Records) != 0 {
			t.Errorf("Expected 0 records after Reset, got %d", len(sink.Records))
		}
		if sink.LastRecord() != nil {
			t.Error("Expected LastRecord to be nil after Reset")
		}
	})

	t.Run("ReturnsError", func(t *testing.T) {
		sink := NewMockAuditSink()
		sink.Err = errors.New("mock error")

		err := sink.Emit(ctx, &AuditRecord{AuditID: "test"})
		if err == nil {
			t.Error("Expected error from MockAuditSink.Emit()")
		}
		if err.Error() != "mock error" {
			t.Errorf("Expected 'mock error', got %v", err)
		}
	})
}

func TestJSONAuditSink(t *testing.T) {
	logger := hermes.NewNoopLogger()
	sink := NewJSONAuditSink(logger)
	ctx := context.Background()

	record := &AuditRecord{
		AuditID:         "test-audit-json",
		Timestamp:       time.Now(),
		SandboxID:       "sandbox-456",
		TemplateID:      "json-template",
		Event:           "sandbox_request_audit",
		ComplianceLevel: "high",
		RetentionPolicy: domain.RetentionPolicy{MaxAge: 24 * time.Hour},
		Metadata:        map[string]string{"env": "prod"},
	}

	err := sink.Emit(ctx, record)
	if err != nil {
		t.Errorf("JSONAuditSink.Emit() unexpected error: %v", err)
	}
}

func TestAuditRecord_FullCycle(t *testing.T) {
	// Test that we can create, emit, and capture a full audit record
	mock := NewMockAuditSink()
	ctx := context.Background()

	record := &AuditRecord{
		AuditID:         "full-cycle-test",
		Timestamp:       time.Now().UTC(),
		SandboxID:       "sandbox-full",
		TemplateID:      "template-full",
		Event:           "sandbox_request_audit",
		ComplianceLevel: "standard",
		RetentionPolicy: domain.RetentionPolicy{
			MaxAge:      2 * time.Hour,
			KeepOutputs: true,
		},
		Metadata: map[string]string{
			"user":        "test-user",
			"application": "test-app",
		},
	}

	err := mock.Emit(ctx, record)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	captured := mock.LastRecord()
	if captured.AuditID != record.AuditID {
		t.Errorf("AuditID mismatch: expected %s, got %s", record.AuditID, captured.AuditID)
	}
	if captured.SandboxID != record.SandboxID {
		t.Errorf("SandboxID mismatch: expected %s, got %s", record.SandboxID, captured.SandboxID)
	}
	if captured.ComplianceLevel != record.ComplianceLevel {
		t.Errorf("ComplianceLevel mismatch: expected %s, got %s", record.ComplianceLevel, captured.ComplianceLevel)
	}
	if captured.Metadata["user"] != "test-user" {
		t.Errorf("Metadata mismatch: expected user=test-user, got %s", captured.Metadata["user"])
	}
}
