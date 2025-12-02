package charon

import "github.com/tartarus-sandbox/tartarus/pkg/hermes"

// metricsAdapter adapts the raw metrics interface to hermes.Metrics.
// This avoids import cycles between charon and hermes.
type metricsAdapter struct {
	raw interface {
		IncCounter(name string, value float64, labels ...interface{})
		ObserveHistogram(name string, value float64, labels ...interface{})
		SetGauge(name string, value float64, labels ...interface{})
	}
}

func (m *metricsAdapter) IncCounter(name string, value float64, labels ...hermes.Label) {
	// Convert hermes.Label to interface{}
	rawLabels := make([]interface{}, len(labels))
	for i, l := range labels {
		rawLabels[i] = l
	}
	m.raw.IncCounter(name, value, rawLabels...)
}

func (m *metricsAdapter) ObserveHistogram(name string, value float64, labels ...hermes.Label) {
	rawLabels := make([]interface{}, len(labels))
	for i, l := range labels {
		rawLabels[i] = l
	}
	m.raw.ObserveHistogram(name, value, rawLabels...)
}

func (m *metricsAdapter) SetGauge(name string, value float64, labels ...hermes.Label) {
	rawLabels := make([]interface{}, len(labels))
	for i, l := range labels {
		rawLabels[i] = l
	}
	m.raw.SetGauge(name, value, rawLabels...)
}
