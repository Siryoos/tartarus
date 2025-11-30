package hermes

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestPrometheusMetrics(t *testing.T) {
	// Reset default registry for testing
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	m := NewPrometheusMetrics()

	// Test Counter
	m.IncCounter("test_counter", 1, Label{Key: "tag", Value: "A"})
	m.IncCounter("test_counter", 2, Label{Key: "tag", Value: "A"})

	// Test Histogram
	m.ObserveHistogram("test_histogram", 0.5, Label{Key: "tag", Value: "B"})

	// Test Gauge
	m.SetGauge("test_gauge", 10, Label{Key: "tag", Value: "C"})
	m.SetGauge("test_gauge", 20, Label{Key: "tag", Value: "C"})

	// Verify metrics exist in the map (internal state check)
	assert.Contains(t, m.counters, "test_counter")
	assert.Contains(t, m.histograms, "test_histogram")
	assert.Contains(t, m.gauges, "test_gauge")
}
