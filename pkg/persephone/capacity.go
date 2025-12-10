package persephone

import (
	"context"
	"math"
	"time"
)

// CapacityOptimizer calculates optimal resource levels
type CapacityOptimizer struct {
	forecaster *HybridForecaster
}

// NewCapacityOptimizer creates a capacity optimizer
func NewCapacityOptimizer() *CapacityOptimizer {
	return &CapacityOptimizer{
		forecaster: NewHybridForecaster(),
	}
}

// CalculateRecommendation determines optimal capacity
func (o *CapacityOptimizer) CalculateRecommendation(
	ctx context.Context,
	history []*UsageRecord,
	targetUtil float64,
	currentSeason *Season,
) (*CapacityRecommendation, error) {

	if len(history) == 0 {
		return &CapacityRecommendation{
			CurrentNodes:     0,
			RecommendedNodes: 1,
			Reason:           "No historical data available",
			ConfidenceLevel:  0.3,
		}, nil
	}

	// Get current usage
	latest := history[len(history)-1]
	currentActive := latest.ActiveVMs

	// Generate short-term forecast (next hour)
	forecast := o.forecaster.Forecast(history, time.Now(), 1*60*60*1000000000, 15*60*1000000000) // 1 hour, 15-min steps

	// Calculate peak predicted demand
	var peakDemand int
	for _, pred := range forecast.Predictions {
		if pred.PredictedDemand > peakDemand {
			peakDemand = pred.PredictedDemand
		}
	}

	// Base recommendation on peak demand
	recommended := int(math.Ceil(float64(peakDemand) / targetUtil))

	// Apply season constraints if available
	reason := "Based on forecasted demand and target utilization"
	if currentSeason != nil {
		if recommended < currentSeason.MinNodes {
			recommended = currentSeason.MinNodes
			reason = "Increased to season minimum"
		} else if recommended > currentSeason.MaxNodes {
			recommended = currentSeason.MaxNodes
			reason = "Capped at season maximum"
		}
	}

	// Ensure minimum of 1
	if recommended < 1 {
		recommended = 1
	}

	// Calculate cost delta (simple linear model)
	costDelta := float64(recommended-currentActive) * 10.0 // $10 per node per hour

	return &CapacityRecommendation{
		CurrentNodes:     currentActive,
		RecommendedNodes: recommended,
		Reason:           reason,
		CostDelta:        costDelta,
		ConfidenceLevel:  forecast.Confidence,
	}, nil
}
