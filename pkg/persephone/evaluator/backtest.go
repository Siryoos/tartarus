package evaluator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/persephone"
)

// Backtester runs historical simulations to evaluate forecast accuracy
type Backtester struct {
	store persephone.HistoryStore
}

// NewBacktester creates a new backtester
func NewBacktester(store persephone.HistoryStore) *Backtester {
	return &Backtester{
		store: store,
	}
}

// Run performs a backtest over the specified period
// trainWindow: duration of history to use for training at each step
// stepSize: how far to move the window forward for each iteration
func (b *Backtester) Run(ctx context.Context, start, end time.Time, trainWindow time.Duration, stepSize time.Duration) (*EvaluationReport, error) {
	// maximize loading range to include training data for the first prediction
	loadStart := start.Add(-trainWindow)
	records, err := b.store.Load(ctx, loadStart, end)
	if err != nil {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	// Sort records by time
	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	report := NewEvaluationReport()
	report.WindowStart = start
	report.WindowEnd = end

	// Iterate through the test window
	forecaster := persephone.NewHybridForecaster()

	// Track accumulated results for metric calculation
	var allPreds []float64
	var allActuals []float64
	var lowerBounds []float64
	var upperBounds []float64

	current := start
	for current.Before(end) {
		// Define training data range
		trainStart := current.Add(-trainWindow)

		// Extract training data
		trainingData := extractRange(records, trainStart, current)

		// Extract actual data for the prediction window (next stepSize)
		// Validation window
		valEnd := current.Add(stepSize)
		if valEnd.After(end) {
			valEnd = end
		}

		// Train model
		forecaster.Train(trainingData)

		// Predict
		// We predict step-by-step within the stepSize to compare with actuals
		// Assuming 15-minute granularity for predictions
		predStep := 15 * time.Minute
		forecast := forecaster.Forecast(trainingData, current, stepSize, predStep)

		// Match predictions with actuals
		for _, pred := range forecast.Predictions {
			// Find actual record for this time
			actual := findRecord(records, pred.Time)
			if actual != nil {
				report.Timestamps = append(report.Timestamps, pred.Time)
				report.Predictions = append(report.Predictions, pred)
				report.Actuals = append(report.Actuals, actual.ActiveVMs)

				allPreds = append(allPreds, float64(pred.PredictedDemand))
				allActuals = append(allActuals, float64(actual.ActiveVMs))
				lowerBounds = append(lowerBounds, float64(pred.LowerBound))
				upperBounds = append(upperBounds, float64(pred.UpperBound))
			}
		}

		current = current.Add(stepSize)
	}

	// Calculate overall metrics
	report.OverallMetrics = CalculateMetrics(allPreds, allActuals, lowerBounds, upperBounds)

	return report, nil
}

func extractRange(records []*persephone.UsageRecord, start, end time.Time) []*persephone.UsageRecord {
	var subset []*persephone.UsageRecord
	for _, r := range records {
		if (r.Timestamp.Equal(start) || r.Timestamp.After(start)) && r.Timestamp.Before(end) {
			subset = append(subset, r)
		}
	}
	return subset
}

func findRecord(records []*persephone.UsageRecord, t time.Time) *persephone.UsageRecord {
	// Simple linear search, optimize if needed
	// Assuming records are sorted helps, but for small step sizes this is fine
	for _, r := range records {
		// Match with some tolerance? Or exact match?
		// Truncate to minute
		if r.Timestamp.Truncate(time.Minute).Equal(t.Truncate(time.Minute)) {
			return r
		}
	}
	return nil
}
