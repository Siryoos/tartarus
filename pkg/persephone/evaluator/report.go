package evaluator

import (
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/persephone"
)

// EvaluationReport contains the results of a backtest
type EvaluationReport struct {
	GeneratedAt time.Time
	WindowStart time.Time
	WindowEnd   time.Time
	ModelConfig string // Description of model parameters

	OverallMetrics MetricResult
	DailyMetrics   map[time.Weekday]MetricResult
	HourlyMetrics  map[int]MetricResult // 0-23

	// Time series data for plotting
	Timestamps  []time.Time
	Actuals     []int
	Predictions []persephone.Prediction
}

// NewEvaluationReport creates an empty report
func NewEvaluationReport() *EvaluationReport {
	return &EvaluationReport{
		GeneratedAt:   time.Now(),
		DailyMetrics:  make(map[time.Weekday]MetricResult),
		HourlyMetrics: make(map[int]MetricResult),
	}
}
