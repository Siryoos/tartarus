package hermes

import "context"

type Label struct {
	Key   string
	Value string
}

type Metrics interface {
	IncCounter(name string, value float64, labels ...Label)
	ObserveHistogram(name string, value float64, labels ...Label)
	SetGauge(name string, value float64, labels ...Label)
}

type Logger interface {
	Info(ctx context.Context, msg string, fields map[string]any)
	Error(ctx context.Context, msg string, fields map[string]any)
}
