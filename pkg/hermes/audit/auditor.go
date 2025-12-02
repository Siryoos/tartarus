package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Auditor records audit events.
type Auditor interface {
	Record(ctx context.Context, event *Event) error
}

// AnomalyDetector analyzes events for anomalies.
type AnomalyDetector interface {
	Analyze(ctx context.Context, event *Event) error
}

// StandardAuditor is the default implementation of Auditor.
type StandardAuditor struct {
	store            Store
	anomalyDetectors []AnomalyDetector
}

// NewStandardAuditor creates a new StandardAuditor.
func NewStandardAuditor(store Store, detectors ...AnomalyDetector) *StandardAuditor {
	return &StandardAuditor{
		store:            store,
		anomalyDetectors: detectors,
	}
}

// Record records the audit event.
func (a *StandardAuditor) Record(ctx context.Context, event *Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 1. Run anomaly detectors (synchronously for now, could be async)
	for _, detector := range a.anomalyDetectors {
		if err := detector.Analyze(ctx, event); err != nil {
			// Log error but don't fail the audit?
			// Or maybe add an annotation to the event?
			// For now, let's just log to stdout or similar fallback,
			// as we don't have a logger passed in here yet.
			// In a real system, we might want to flag the event as suspicious.
			if event.Metadata == nil {
				event.Metadata = make(map[string]interface{})
			}
			event.Metadata["anomaly_error"] = err.Error()
		}
	}

	// 2. Write to store
	if err := a.store.Write(ctx, event); err != nil {
		return fmt.Errorf("failed to write audit event: %w", err)
	}

	return nil
}
