package hypnos

import (
	"context"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// trace starts a trace span and returns a function to end it.
// It logs the start and end of the operation with duration.
func (m *Manager) trace(ctx context.Context, name string) func() {
	start := time.Now()
	if m.Metrics != nil {
		// We can't easily log "start" with the current Metrics interface if it only supports counters/histograms.
		// But if we had a logger, we would log here.
		// For now, we'll just track the start time.
	}

	return func() {
		duration := time.Since(start)
		if m.Metrics != nil {
			m.Metrics.ObserveHistogram("hypnos_trace_duration_seconds", duration.Seconds(), hermes.Label{Key: "span", Value: name})
		}
		// In a real system, we would log "completed" here with the duration.
		// fmt.Printf("Trace: %s took %v\n", name, duration) // Debug logging
	}
}
