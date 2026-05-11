package acheron

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// metricsQueue is a transparent decorator that wraps any Queue implementation
// and emits Prometheus-compatible metrics for every operation.
// This ensures that MemoryQueue, PriorityQueue, and any future Queue
// implementations share an identical telemetry surface with RedisQueue.
type metricsQueue struct {
	inner     Queue
	metrics   hermes.Metrics
	queueName string
}

// NewInstrumentedQueue wraps q with counter/gauge instrumentation using the
// provided Metrics sink. The queueName label is attached to every metric so
// multiple queues can be distinguished in dashboards.
//
//	q := acheron.NewInstrumentedQueue(
//	    acheron.NewMemoryQueue(hermes.NewNoopMetrics(), ""),
//	    prometheusMetrics,
//	    "tartarus:jobs",
//	)
func NewInstrumentedQueue(q Queue, metrics hermes.Metrics, queueName string) Queue {
	return &metricsQueue{inner: q, metrics: metrics, queueName: queueName}
}

func (m *metricsQueue) label() hermes.Label {
	return hermes.Label{Key: "queue", Value: m.queueName}
}

func (m *metricsQueue) Enqueue(ctx context.Context, req *domain.SandboxRequest) error {
	err := m.inner.Enqueue(ctx, req)
	if err != nil {
		m.metrics.IncCounter("queue_enqueue_errors_total", 1, m.label())
		return err
	}
	m.metrics.IncCounter("queue_enqueue_total", 1, m.label())
	m.metrics.SetGauge("queue_depth", float64(m.inner.Len(ctx)), m.label())
	return nil
}

func (m *metricsQueue) Dequeue(ctx context.Context) (*domain.SandboxRequest, string, error) {
	req, receipt, err := m.inner.Dequeue(ctx)
	if err != nil {
		return nil, "", err
	}
	m.metrics.IncCounter("queue_dequeue_total", 1, m.label())
	m.metrics.SetGauge("queue_depth", float64(m.inner.Len(ctx)), m.label())
	return req, receipt, nil
}

func (m *metricsQueue) Ack(ctx context.Context, receipt string) error {
	return m.inner.Ack(ctx, receipt)
}

func (m *metricsQueue) Nack(ctx context.Context, receipt string, reason string) error {
	err := m.inner.Nack(ctx, receipt, reason)
	if err != nil {
		m.metrics.IncCounter("queue_nack_errors_total", 1, m.label())
		return err
	}
	m.metrics.IncCounter("queue_nack_total", 1, m.label())
	return nil
}

func (m *metricsQueue) Len(ctx context.Context) int {
	return m.inner.Len(ctx)
}
