package evaluator

import (
	"math"
)

// MetricResult holds the calculated error metrics
type MetricResult struct {
	MAE      float64 // Mean Absolute Error
	RMSE     float64 // Root Mean Square Error
	MAPE     float64 // Mean Absolute Percentage Error
	Coverage float64 // Percentage of actuals within prediction intervals
}

// CalculateMetrics computes accuracy metrics for predictions vs actuals
// predictions and actuals must be sorted by time and aligned
func CalculateMetrics(predictions []float64, actuals []float64, lowerBounds, upperBounds []float64) MetricResult {
	if len(predictions) != len(actuals) || len(predictions) == 0 {
		return MetricResult{}
	}

	var sumAbsError float64
	var sumSquaredError float64
	var sumPctError float64
	var coverageCount int

	n := float64(len(predictions))

	for i := 0; i < len(predictions); i++ {
		pred := predictions[i]
		act := actuals[i]

		err := act - pred
		absErr := math.Abs(err)

		sumAbsError += absErr
		sumSquaredError += err * err

		if act != 0 {
			sumPctError += absErr / act
		}

		// Check coverage if bounds provided
		if i < len(lowerBounds) && i < len(upperBounds) {
			if act >= lowerBounds[i] && act <= upperBounds[i] {
				coverageCount++
			}
		}
	}

	result := MetricResult{
		MAE:  sumAbsError / n,
		RMSE: math.Sqrt(sumSquaredError / n),
		MAPE: (sumPctError / n) * 100.0,
	}

	if len(lowerBounds) > 0 {
		result.Coverage = (float64(coverageCount) / n) * 100.0
	}

	return result
}
