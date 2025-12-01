package persephone

import (
	"math"
	"time"
)

// PatternDetector identifies recurring patterns in usage data
type PatternDetector struct {
	hourlyBuckets  [24]float64 // Average demand per hour of day
	dailyBuckets   [7]float64  // Average demand per day of week
	weeklyBaseline float64     // Overall weekly average
}

// NewPatternDetector creates a pattern detector
func NewPatternDetector() *PatternDetector {
	return &PatternDetector{}
}

// AnalyzePatterns detects recurring patterns from historical data
func (p *PatternDetector) AnalyzePatterns(history []*UsageRecord) *PatternAnalysis {
	if len(history) == 0 {
		return &PatternAnalysis{}
	}

	// Reset buckets
	p.hourlyBuckets = [24]float64{}
	p.dailyBuckets = [7]float64{}
	p.weeklyBaseline = 0

	hourlyCounts := make([]int, 24)
	dailyCounts := make([]int, 7)
	var totalDemand float64
	var count int

	for _, record := range history {
		hour := record.Timestamp.Hour()
		weekday := int(record.Timestamp.Weekday())
		demand := float64(record.ActiveVMs)

		p.hourlyBuckets[hour] += demand
		hourlyCounts[hour]++

		p.dailyBuckets[weekday] += demand
		dailyCounts[weekday]++

		totalDemand += demand
		count++
	}

	// Calculate averages
	for i := 0; i < 24; i++ {
		if hourlyCounts[i] > 0 {
			p.hourlyBuckets[i] /= float64(hourlyCounts[i])
		}
	}
	for i := 0; i < 7; i++ {
		if dailyCounts[i] > 0 {
			p.dailyBuckets[i] /= float64(dailyCounts[i])
		}
	}
	if count > 0 {
		p.weeklyBaseline = totalDemand / float64(count)
	}

	// Detect peaks and confidence
	analysis := &PatternAnalysis{
		HourlyPattern: p.hourlyBuckets[:],
		DailyPattern:  p.dailyBuckets[:],
		Baseline:      p.weeklyBaseline,
	}

	// Find peak hours (above 120% of baseline)
	threshold := p.weeklyBaseline * 1.2
	for hour, demand := range p.hourlyBuckets {
		if demand >= threshold {
			analysis.PeakHours = append(analysis.PeakHours, hour)
		}
	}

	// Calculate pattern strength (variance)
	analysis.Confidence = p.calculateConfidence(history)

	return analysis
}

func (p *PatternDetector) calculateConfidence(history []*UsageRecord) float64 {
	if len(history) < 10 {
		return 0.3 // Low confidence with little data
	}

	// Calculate variance from predicted pattern
	var squaredErrors float64
	for _, record := range history {
		hour := record.Timestamp.Hour()
		weekday := int(record.Timestamp.Weekday())

		// Combine hourly and daily patterns
		predicted := (p.hourlyBuckets[hour] + p.dailyBuckets[weekday]) / 2
		actual := float64(record.ActiveVMs)

		error := actual - predicted
		squaredErrors += error * error
	}

	mse := squaredErrors / float64(len(history))

	// Convert MSE to confidence (lower error = higher confidence)
	// Using 1/(1+sqrt(mse)) to normalize to 0-1 range
	confidence := 1.0 / (1.0 + math.Sqrt(mse))

	return math.Min(confidence, 0.95) // Cap at 0.95
}

// PredictDemand predicts demand for a specific time using patterns
func (p *PatternDetector) PredictDemand(t time.Time) (demand float64, confidence float64) {
	hour := t.Hour()
	weekday := int(t.Weekday())

	// Weight hourly pattern more heavily (70/30 split)
	hourlyWeight := 0.7
	dailyWeight := 0.3

	predicted := (p.hourlyBuckets[hour] * hourlyWeight) +
		(p.dailyBuckets[weekday] * dailyWeight)

	// Use baseline if patterns are weak
	if predicted < p.weeklyBaseline*0.5 {
		predicted = p.weeklyBaseline
	}

	return predicted, 0.8 // TODO: Calculate actual confidence
}

// PatternAnalysis contains detected patterns
type PatternAnalysis struct {
	HourlyPattern []float64 // Demand per hour (0-23)
	DailyPattern  []float64 // Demand per weekday (Sunday=0)
	Baseline      float64   // Overall average
	PeakHours     []int     // Hours with peak demand
	Confidence    float64   // 0-1, how reliable the pattern is
}

// ExponentialSmoothingPredictor uses exponential smoothing for forecasting
type ExponentialSmoothingPredictor struct {
	alpha float64 // Smoothing factor (0-1)
	level float64 // Current level estimate
}

// NewExponentialSmoothingPredictor creates a predictor with given smoothing factor
func NewExponentialSmoothingPredictor(alpha float64) *ExponentialSmoothingPredictor {
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.3 // Default
	}
	return &ExponentialSmoothingPredictor{
		alpha: alpha,
	}
}

// Train initializes the predictor with historical data
func (p *ExponentialSmoothingPredictor) Train(history []*UsageRecord) {
	if len(history) == 0 {
		return
	}

	// Initialize level with first value
	p.level = float64(history[0].ActiveVMs)

	// Apply exponential smoothing
	for _, record := range history[1:] {
		observed := float64(record.ActiveVMs)
		p.level = p.alpha*observed + (1-p.alpha)*p.level
	}
}

// Predict forecasts the next value
func (p *ExponentialSmoothingPredictor) Predict() float64 {
	return p.level
}

// Update updates the model with a new observation
func (p *ExponentialSmoothingPredictor) Update(actual float64) {
	p.level = p.alpha*actual + (1-p.alpha)*p.level
}

// ConfidenceCalculator computes prediction intervals
type ConfidenceCalculator struct{}

// NewConfidenceCalculator creates a confidence calculator
func NewConfidenceCalculator() *ConfidenceCalculator {
	return &ConfidenceCalculator{}
}

// CalculateInterval computes prediction interval bounds
func (c *ConfidenceCalculator) CalculateInterval(history []*UsageRecord, prediction float64) (lower, upper float64) {
	if len(history) < 2 {
		// Wide interval with little data
		return prediction * 0.5, prediction * 1.5
	}

	// Calculate historical variance
	var values []float64
	for _, record := range history {
		values = append(values, float64(record.ActiveVMs))
	}

	stdDev := c.standardDeviation(values)

	// 95% confidence interval (approximately ±2 standard deviations)
	margin := 2.0 * stdDev

	lower = math.Max(0, prediction-margin)
	upper = prediction + margin

	return lower, upper
}

func (c *ConfidenceCalculator) standardDeviation(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Calculate mean
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Calculate variance
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))

	return math.Sqrt(variance)
}

// HybridForecaster combines pattern detection and exponential smoothing
type HybridForecaster struct {
	patternDetector *PatternDetector
	smoothing       *ExponentialSmoothingPredictor
	confidence      *ConfidenceCalculator
}

// NewHybridForecaster creates a hybrid forecasting model
func NewHybridForecaster() *HybridForecaster {
	return &HybridForecaster{
		patternDetector: NewPatternDetector(),
		smoothing:       NewExponentialSmoothingPredictor(0.3),
		confidence:      NewConfidenceCalculator(),
	}
}

// Train trains the forecaster with historical data
func (f *HybridForecaster) Train(history []*UsageRecord) {
	if len(history) == 0 {
		return
	}

	// Train both models
	f.patternDetector.AnalyzePatterns(history)
	f.smoothing.Train(history)
}

// Forecast generates predictions for a time window
func (f *HybridForecaster) Forecast(history []*UsageRecord, window time.Duration, stepInterval time.Duration) *Forecast {
	if len(history) == 0 {
		return &Forecast{
			GeneratedAt: time.Now(),
			Window:      window,
			Predictions: []Prediction{},
			Confidence:  0.0,
		}
	}

	// Train models
	f.Train(history)

	// Generate predictions
	predictions := []Prediction{}
	now := time.Now()
	steps := int(window / stepInterval)

	for i := 0; i < steps; i++ {
		t := now.Add(time.Duration(i) * stepInterval)

		// Get pattern-based prediction
		patternDemand, patternConf := f.patternDetector.PredictDemand(t)

		// Get smoothing-based prediction
		smoothingDemand := f.smoothing.Predict()

		// Combine predictions (weighted average)
		combined := (patternDemand * 0.6) + (smoothingDemand * 0.4)

		// Calculate confidence interval
		lower, upper := f.confidence.CalculateInterval(history, combined)

		predictions = append(predictions, Prediction{
			Time:            t,
			PredictedDemand: int(math.Round(combined)),
			UpperBound:      int(math.Ceil(upper)),
			LowerBound:      int(math.Floor(lower)),
			Confidence:      patternConf,
		})
	}

	// Overall forecast confidence
	analysis := f.patternDetector.AnalyzePatterns(history)

	return &Forecast{
		GeneratedAt: now,
		Window:      window,
		Predictions: predictions,
		Confidence:  analysis.Confidence,
	}
}
