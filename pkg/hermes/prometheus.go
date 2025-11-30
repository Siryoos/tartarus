package hermes

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// PrometheusMetrics implements the Metrics interface using Prometheus.
type PrometheusMetrics struct {
	counters   map[string]*prometheus.CounterVec
	histograms map[string]*prometheus.HistogramVec
	gauges     map[string]*prometheus.GaugeVec
	mu         sync.RWMutex
}

// NewPrometheusMetrics creates a new PrometheusMetrics instance.
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		counters:   make(map[string]*prometheus.CounterVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
	}
}

func (m *PrometheusMetrics) getLabels(labels []Label) ([]string, []string) {
	keys := make([]string, len(labels))
	values := make([]string, len(labels))
	for i, l := range labels {
		keys[i] = l.Key
		values[i] = l.Value
	}
	return keys, values
}

func (m *PrometheusMetrics) IncCounter(name string, value float64, labels ...Label) {
	m.mu.RLock()
	vec, ok := m.counters[name]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		// Double check
		vec, ok = m.counters[name]
		if !ok {
			keys, _ := m.getLabels(labels)
			vec = prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: name,
				Help: name,
			}, keys)
			prometheus.MustRegister(vec)
			m.counters[name] = vec
		}
		m.mu.Unlock()
	}

	_, values := m.getLabels(labels)
	vec.WithLabelValues(values...).Add(value)
}

func (m *PrometheusMetrics) ObserveHistogram(name string, value float64, labels ...Label) {
	m.mu.RLock()
	vec, ok := m.histograms[name]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		vec, ok = m.histograms[name]
		if !ok {
			keys, _ := m.getLabels(labels)
			vec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Name: name,
				Help: name,
			}, keys)
			prometheus.MustRegister(vec)
			m.histograms[name] = vec
		}
		m.mu.Unlock()
	}

	_, values := m.getLabels(labels)
	vec.WithLabelValues(values...).Observe(value)
}

func (m *PrometheusMetrics) SetGauge(name string, value float64, labels ...Label) {
	m.mu.RLock()
	vec, ok := m.gauges[name]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		vec, ok = m.gauges[name]
		if !ok {
			keys, _ := m.getLabels(labels)
			vec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: name,
				Help: name,
			}, keys)
			prometheus.MustRegister(vec)
			m.gauges[name] = vec
		}
		m.mu.Unlock()
	}

	_, values := m.getLabels(labels)
	vec.WithLabelValues(values...).Set(value)
}
