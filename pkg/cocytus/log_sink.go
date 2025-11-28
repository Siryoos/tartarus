package cocytus

import (
	"context"
	"log/slog"
)

// LogSink is a simple Dead Letter Sink that logs failed jobs to the standard logger.
type LogSink struct {
	logger *slog.Logger
}

// NewLogSink creates a new LogSink.
func NewLogSink(logger *slog.Logger) *LogSink {
	return &LogSink{
		logger: logger,
	}
}

// Write logs the dead letter record at ERROR level.
func (s *LogSink) Write(ctx context.Context, rec *Record) error {
	s.logger.Error("Dead letter received",
		"run_id", rec.RunID,
		"request_id", rec.RequestID,
		"reason", rec.Reason,
		"created_at", rec.CreatedAt,
		"payload_size", len(rec.Payload),
	)
	return nil
}
