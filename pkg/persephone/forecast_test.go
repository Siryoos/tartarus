package persephone

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatternDetector_HourlyPattern(t *testing.T) {
	detector := NewPatternDetector()

	// Create synthetic data with peak at hour 14 (2 PM) over 4 weeks
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	history := []*UsageRecord{}

	hourCounter := 0
	for week := 0; week < 4; week++ {
		for day := 0; day < 7; day++ {
			for hour := 0; hour < 24; hour++ {
				ts := baseTime.Add(time.Duration(hourCounter) * time.Hour)
				demand := 10
				if hour == 14 { // Peak at 2 PM
					demand = 50
				}
				history = append(history, &UsageRecord{
					Timestamp: ts,
					ActiveVMs: demand,
				})
				hourCounter++
			}
		}
	}

	analysis := detector.AnalyzePatterns(history)

	// Verify hour 14 shows elevated demand (should be 50, others should be 10)
	assert.Equal(t, 50.0, analysis.HourlyPattern[14], "Hour 14 should have demand of 50")
	assert.Equal(t, 10.0, analysis.HourlyPattern[10], "Hour 10 should have demand of 10")

	// Baseline calculation: (50 + 23*10) * 28 days / (28 * 24) hours ≈ 11.67
	assert.Greater(t, analysis.Baseline, 10.0)
	assert.Less(t, analysis.Baseline, 15.0)
}

func TestPatternDetector_DailyPattern(t *testing.T) {
	detector := NewPatternDetector()

	// Create synthetic data with higher demand on weekdays
	now := time.Now().Truncate(time.Hour)
	history := []*UsageRecord{}

	for day := 0; day < 28; day++ { // 4 weeks
		ts := now.Add(time.Duration(day*24) * time.Hour)
		weekday := ts.Weekday()
		demand := 10
		if weekday >= time.Monday && weekday <= time.Friday {
			demand = 30 // Weekday traffic
		}
		history = append(history, &UsageRecord{
			Timestamp: ts,
			ActiveVMs: demand,
		})
	}

	analysis := detector.AnalyzePatterns(history)

	// Verify weekday pattern
	assert.Greater(t, analysis.DailyPattern[int(time.Monday)], analysis.DailyPattern[int(time.Sunday)])
	assert.Greater(t, analysis.DailyPattern[int(time.Wednesday)], analysis.DailyPattern[int(time.Saturday)])
}

func TestPatternDetector_PredictDemand(t *testing.T) {
	detector := NewPatternDetector()

	// Train with some data
	now := time.Now().Truncate(time.Hour)
	history := []*UsageRecord{
		{Timestamp: now.Add(-2 * time.Hour), ActiveVMs: 10},
		{Timestamp: now.Add(-1 * time.Hour), ActiveVMs: 20},
		{Timestamp: now, ActiveVMs: 30},
	}

	detector.AnalyzePatterns(history)

	// Predict for next hour
	demand, confidence := detector.PredictDemand(now.Add(time.Hour))
	assert.Greater(t, demand, 0.0)
	assert.Greater(t, confidence, 0.0)
	assert.LessOrEqual(t, confidence, 1.0)
}

func TestExponentialSmoothingPredictor(t *testing.T) {
	predictor := NewExponentialSmoothingPredictor(0.3)

	// Train with increasing trend
	history := []*UsageRecord{
		{Timestamp: time.Now(), ActiveVMs: 10},
		{Timestamp: time.Now(), ActiveVMs: 15},
		{Timestamp: time.Now(), ActiveVMs: 20},
		{Timestamp: time.Now(), ActiveVMs: 25},
	}

	predictor.Train(history)

	// Prediction should be close to recent values
	prediction := predictor.Predict()
	assert.Greater(t, prediction, 15.0)
	assert.Less(t, prediction, 30.0)

	// Update with new observation
	predictor.Update(30)
	newPrediction := predictor.Predict()
	assert.Greater(t, newPrediction, prediction) // Should increase
}

func TestConfidenceCalculator_Interval(t *testing.T) {
	calc := NewConfidenceCalculator()

	// History with low variance
	history := []*UsageRecord{
		{Timestamp: time.Now(), ActiveVMs: 10},
		{Timestamp: time.Now(), ActiveVMs: 11},
		{Timestamp: time.Now(), ActiveVMs: 10},
		{Timestamp: time.Now(), ActiveVMs: 9},
		{Timestamp: time.Now(), ActiveVMs: 10},
	}

	lower, upper := calc.CalculateInterval(history, 10.0)

	// Bounds should be reasonable
	assert.Greater(t, lower, 0.0)
	assert.Less(t, upper, 20.0)
	assert.Greater(t, upper, lower)

	// Interval should be tighter for low-variance data
	intervalWidth := upper - lower
	assert.Less(t, intervalWidth, 10.0)
}

func TestConfidenceCalculator_EmptyHistory(t *testing.T) {
	calc := NewConfidenceCalculator()

	lower, upper := calc.CalculateInterval([]*UsageRecord{}, 20.0)

	// Should provide wide default interval
	assert.Equal(t, 10.0, lower) // 20 * 0.5
	assert.Equal(t, 30.0, upper) // 20 * 1.5
}

func TestHybridForecaster(t *testing.T) {
	forecaster := NewHybridForecaster()

	// Create realistic usage pattern
	now := time.Now().Truncate(time.Hour)
	history := []*UsageRecord{}

	for i := 0; i < 168; i++ { // 1 week of hourly data
		ts := now.Add(time.Duration(-168+i) * time.Hour)
		hour := ts.Hour()

		// Simulate business hours pattern
		demand := 5                  // Baseline
		if hour >= 9 && hour <= 17 { // Business hours
			demand = 20
		}

		history = append(history, &UsageRecord{
			Timestamp: ts,
			ActiveVMs: demand,
		})
	}

	// Generate 6-hour forecast
	forecast := forecaster.Forecast(history, 6*time.Hour, 1*time.Hour)

	require.NotNil(t, forecast)
	assert.Equal(t, 6*time.Hour, forecast.Window)
	assert.Equal(t, 6, len(forecast.Predictions))
	assert.Greater(t, forecast.Confidence, 0.0)

	// Verify predictions have bounds
	for _, pred := range forecast.Predictions {
		assert.Greater(t, pred.PredictedDemand, 0)
		assert.Greater(t, pred.UpperBound, pred.PredictedDemand)
		assert.Less(t, pred.LowerBound, pred.PredictedDemand)
		assert.GreaterOrEqual(t, pred.LowerBound, 0)
	}
}

func TestHybridForecaster_EmptyHistory(t *testing.T) {
	forecaster := NewHybridForecaster()

	forecast := forecaster.Forecast([]*UsageRecord{}, 24*time.Hour, 1*time.Hour)

	require.NotNil(t, forecast)
	assert.Equal(t, 0, len(forecast.Predictions))
	assert.Equal(t, 0.0, forecast.Confidence)
}

func BenchmarkForecast(b *testing.B) {
	forecaster := NewHybridForecaster()

	// Create 10k records (simulating 90 days @ 10-minute intervals)
	now := time.Now()
	history := make([]*UsageRecord, 10000)
	for i := 0; i < 10000; i++ {
		history[i] = &UsageRecord{
			Timestamp: now.Add(time.Duration(-10000+i) * 10 * time.Minute),
			ActiveVMs: 10 + (i % 50), // Some variation
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		forecaster.Forecast(history, 24*time.Hour, 5*time.Minute)
	}
}
