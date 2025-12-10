// Package perf provides a repeatable performance testing harness with Hermes metrics integration.
package perf

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// SLOTarget defines a Service Level Objective for performance testing.
type SLOTarget struct {
	Name        string
	MetricName  string
	Target      time.Duration
	Percentile  int // 0 for average, 50 for p50, 95 for p95, 99 for p99
	Description string
}

// Phase4SLOs defines all SLO targets for Phase 4.
var Phase4SLOs = []SLOTarget{
	{
		Name:        "PythonDS Cold Start",
		MetricName:  "perf_python_ds_cold_start_seconds",
		Target:      200 * time.Millisecond,
		Percentile:  99,
		Description: "Python data science sandbox cold start latency",
	},
	{
		Name:        "OCI Image Conversion",
		MetricName:  "perf_erebus_oci_conversion_seconds",
		Target:      30 * time.Second,
		Percentile:  99,
		Description: "OCI image to rootfs conversion time",
	},
	{
		Name:        "Typhon Quarantine Routing",
		MetricName:  "perf_typhon_quarantine_overhead_seconds",
		Target:      50 * time.Millisecond,
		Percentile:  99,
		Description: "Additional latency for quarantine routing vs normal",
	},
	{
		Name:        "Hypnos Wake From Sleep",
		MetricName:  "perf_hypnos_wake_seconds",
		Target:      100 * time.Millisecond,
		Percentile:  99,
		Description: "Time to wake a hibernated sandbox",
	},
}

// PerfResult represents a single performance measurement.
type PerfResult struct {
	Name      string
	Duration  time.Duration
	Labels    map[string]string
	Timestamp time.Time
	Passed    bool
	Error     error
}

// PerfHarness collects performance metrics and checks SLO compliance.
type PerfHarness struct {
	metrics    hermes.Metrics
	results    []PerfResult
	sloTargets map[string]SLOTarget
	mu         sync.Mutex
}

// NewPerfHarness creates a new performance testing harness.
func NewPerfHarness(metrics hermes.Metrics) *PerfHarness {
	targets := make(map[string]SLOTarget)
	for _, slo := range Phase4SLOs {
		targets[slo.MetricName] = slo
	}
	return &PerfHarness{
		metrics:    metrics,
		results:    make([]PerfResult, 0),
		sloTargets: targets,
	}
}

// RecordResult records a performance measurement.
func (h *PerfHarness) RecordResult(metricName string, duration time.Duration, labels map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	slo, hasSLO := h.sloTargets[metricName]
	passed := true
	if hasSLO {
		passed = duration <= slo.Target
	}

	result := PerfResult{
		Name:      metricName,
		Duration:  duration,
		Labels:    labels,
		Timestamp: time.Now(),
		Passed:    passed,
	}
	h.results = append(h.results, result)

	// Record to Hermes metrics
	if h.metrics != nil {
		metricLabels := make([]hermes.Label, 0, len(labels))
		for k, v := range labels {
			metricLabels = append(metricLabels, hermes.Label{Key: k, Value: v})
		}
		h.metrics.ObserveHistogram(metricName, duration.Seconds(), metricLabels...)

		// Track SLO compliance
		compliance := 0.0
		if passed {
			compliance = 1.0
		}
		h.metrics.SetGauge(metricName+"_slo_compliance", compliance, metricLabels...)
	}
}

// RecordError records a failed measurement.
func (h *PerfHarness) RecordError(metricName string, err error, labels map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	result := PerfResult{
		Name:      metricName,
		Timestamp: time.Now(),
		Passed:    false,
		Error:     err,
		Labels:    labels,
	}
	h.results = append(h.results, result)

	if h.metrics != nil {
		metricLabels := make([]hermes.Label, 0, len(labels)+1)
		for k, v := range labels {
			metricLabels = append(metricLabels, hermes.Label{Key: k, Value: v})
		}
		h.metrics.IncCounter(metricName+"_errors_total", 1, metricLabels...)
	}
}

// getResultsLocked returns all recorded results for a metric without acquiring the lock.
// Caller must hold h.mu.
func (h *PerfHarness) getResultsLocked(metricName string) []PerfResult {
	var filtered []PerfResult
	for _, r := range h.results {
		if r.Name == metricName {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// GetResults returns all recorded results for a metric.
func (h *PerfHarness) GetResults(metricName string) []PerfResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.getResultsLocked(metricName)
}

// CalculatePercentile calculates the given percentile for a metric.
func (h *PerfHarness) CalculatePercentile(metricName string, percentile int) (time.Duration, error) {
	results := h.GetResults(metricName)
	if len(results) == 0 {
		return 0, fmt.Errorf("no results for metric %s", metricName)
	}

	durations := make([]time.Duration, 0, len(results))
	for _, r := range results {
		if r.Error == nil {
			durations = append(durations, r.Duration)
		}
	}

	if len(durations) == 0 {
		return 0, fmt.Errorf("no successful results for metric %s", metricName)
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	index := (percentile * len(durations)) / 100
	if index >= len(durations) {
		index = len(durations) - 1
	}

	return durations[index], nil
}

// CheckSLO checks if a metric meets its SLO target.
func (h *PerfHarness) CheckSLO(metricName string) (bool, string) {
	slo, exists := h.sloTargets[metricName]
	if !exists {
		return true, "no SLO defined"
	}

	p, err := h.CalculatePercentile(metricName, slo.Percentile)
	if err != nil {
		return false, fmt.Sprintf("error calculating percentile: %v", err)
	}

	if p > slo.Target {
		return false, fmt.Sprintf("P%d latency %v exceeds target %v", slo.Percentile, p, slo.Target)
	}

	return true, fmt.Sprintf("P%d latency %v meets target %v", slo.Percentile, p, slo.Target)
}

// GenerateReport generates a summary report of all SLOs.
func (h *PerfHarness) GenerateReport() *SLOReport {
	h.mu.Lock()
	defer h.mu.Unlock()

	report := &SLOReport{
		GeneratedAt: time.Now(),
		SLOResults:  make([]SLOResult, 0, len(h.sloTargets)),
	}

	for metricName, slo := range h.sloTargets {
		results := h.getResultsLocked(metricName)

		var durations []time.Duration
		var errors int
		for _, r := range results {
			if r.Error != nil {
				errors++
			} else {
				durations = append(durations, r.Duration)
			}
		}

		var p50, p95, p99, avg time.Duration
		var min, max time.Duration
		if len(durations) > 0 {
			sort.Slice(durations, func(i, j int) bool {
				return durations[i] < durations[j]
			})
			p50 = durations[len(durations)*50/100]
			p95 = durations[len(durations)*95/100]
			if len(durations)*99/100 < len(durations) {
				p99 = durations[len(durations)*99/100]
			} else {
				p99 = durations[len(durations)-1]
			}
			min = durations[0]
			max = durations[len(durations)-1]

			var sum time.Duration
			for _, d := range durations {
				sum += d
			}
			avg = sum / time.Duration(len(durations))
		}

		var targetP time.Duration
		switch slo.Percentile {
		case 50:
			targetP = p50
		case 95:
			targetP = p95
		case 99:
			targetP = p99
		default:
			targetP = avg
		}

		passed := targetP <= slo.Target || len(durations) == 0

		result := SLOResult{
			Name:        slo.Name,
			MetricName:  metricName,
			Target:      slo.Target,
			Percentile:  slo.Percentile,
			Actual:      targetP,
			Passed:      passed,
			SampleCount: len(durations),
			ErrorCount:  errors,
			Stats: LatencyStats{
				Min: min,
				Max: max,
				Avg: avg,
				P50: p50,
				P95: p95,
				P99: p99,
			},
		}
		report.SLOResults = append(report.SLOResults, result)
	}

	// Sort by name for consistent ordering
	sort.Slice(report.SLOResults, func(i, j int) bool {
		return report.SLOResults[i].Name < report.SLOResults[j].Name
	})

	return report
}

// SLOReport contains a summary of all SLO checks.
type SLOReport struct {
	GeneratedAt time.Time
	SLOResults  []SLOResult
}

// SLOResult contains the result of a single SLO check.
type SLOResult struct {
	Name        string
	MetricName  string
	Target      time.Duration
	Percentile  int
	Actual      time.Duration
	Passed      bool
	SampleCount int
	ErrorCount  int
	Stats       LatencyStats
}

// LatencyStats contains latency distribution statistics.
type LatencyStats struct {
	Min time.Duration
	Max time.Duration
	Avg time.Duration
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
}

// String returns a formatted string representation of the report.
func (r *SLOReport) String() string {
	var output string
	output += fmt.Sprintf("=== Phase 4 SLO Report ===\n")
	output += fmt.Sprintf("Generated: %s\n\n", r.GeneratedAt.Format(time.RFC3339))

	passCount := 0
	for _, result := range r.SLOResults {
		if result.Passed {
			passCount++
		}
	}
	output += fmt.Sprintf("Summary: %d/%d SLOs passing\n\n", passCount, len(r.SLOResults))

	for _, result := range r.SLOResults {
		status := "PASS"
		if !result.Passed {
			status = "FAIL"
		}
		output += fmt.Sprintf("[%s] %s\n", status, result.Name)
		output += fmt.Sprintf("  Metric: %s\n", result.MetricName)
		output += fmt.Sprintf("  Target: P%d < %v\n", result.Percentile, result.Target)
		output += fmt.Sprintf("  Actual: P%d = %v\n", result.Percentile, result.Actual)
		output += fmt.Sprintf("  Samples: %d (errors: %d)\n", result.SampleCount, result.ErrorCount)
		if result.SampleCount > 0 {
			output += fmt.Sprintf("  Stats: min=%v avg=%v max=%v p50=%v p95=%v p99=%v\n",
				result.Stats.Min, result.Stats.Avg, result.Stats.Max,
				result.Stats.P50, result.Stats.P95, result.Stats.P99)
		}
		output += "\n"
	}

	return output
}

// Timer is a helper for timing operations.
type Timer struct {
	harness    *PerfHarness
	metricName string
	labels     map[string]string
	start      time.Time
}

// StartTimer starts a new timer for the given metric.
func (h *PerfHarness) StartTimer(metricName string, labels map[string]string) *Timer {
	return &Timer{
		harness:    h,
		metricName: metricName,
		labels:     labels,
		start:      time.Now(),
	}
}

// Stop stops the timer and records the duration.
func (t *Timer) Stop() time.Duration {
	duration := time.Since(t.start)
	t.harness.RecordResult(t.metricName, duration, t.labels)
	return duration
}

// StopWithError stops the timer and records an error.
func (t *Timer) StopWithError(err error) {
	t.harness.RecordError(t.metricName, err, t.labels)
}
