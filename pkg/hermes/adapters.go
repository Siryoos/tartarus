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

type NoopLogger struct{}

func NewNoopLogger() *NoopLogger {
	return &NoopLogger{}
}

func (l *NoopLogger) Info(ctx context.Context, msg string, fields map[string]any)  {}
func (l *NoopLogger) Error(ctx context.Context, msg string, fields map[string]any) {}

type LogMetrics struct {
	logger *slog.Logger
}

func NewLogMetrics() *LogMetrics {
	return &LogMetrics{
		logger: slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
}

func (m *LogMetrics) IncCounter(name string, value float64, labels ...Label) {
	args := make([]any, 0, len(labels)*2+2)
	args = append(args, "metric", name, "value", value, "type", "counter")
	for _, l := range labels {
		args = append(args, l.Key, l.Value)
	}
	m.logger.Info("Metric", args...)
}

func (m *LogMetrics) ObserveHistogram(name string, value float64, labels ...Label) {
	args := make([]any, 0, len(labels)*2+2)
	args = append(args, "metric", name, "value", value, "type", "histogram")
	for _, l := range labels {
		args = append(args, l.Key, l.Value)
	}
	m.logger.Info("Metric", args...)
}

func (m *LogMetrics) SetGauge(name string, value float64, labels ...Label) {
	args := make([]any, 0, len(labels)*2+2)
	args = append(args, "metric", name, "value", value, "type", "gauge")
	for _, l := range labels {
		args = append(args, l.Key, l.Value)
	}
	m.logger.Info("Metric", args...)
}
