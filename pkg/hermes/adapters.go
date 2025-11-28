package hermes

import (
	"context"
	"log/slog"
	"os"
)

type SlogAdapter struct {
	logger *slog.Logger
}

func NewSlogAdapter() *SlogAdapter {
	return &SlogAdapter{
		logger: slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
}

func (l *SlogAdapter) Info(ctx context.Context, msg string, fields map[string]any) {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	l.logger.InfoContext(ctx, msg, args...)
}

func (l *SlogAdapter) Error(ctx context.Context, msg string, fields map[string]any) {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	l.logger.ErrorContext(ctx, msg, args...)
}

type NoopMetrics struct{}

func NewNoopMetrics() *NoopMetrics {
	return &NoopMetrics{}
}

func (m *NoopMetrics) IncCounter(name string, value float64, labels ...Label)       {}
func (m *NoopMetrics) ObserveHistogram(name string, value float64, labels ...Label) {}
func (m *NoopMetrics) SetGauge(name string, value float64, labels ...Label)         {}
