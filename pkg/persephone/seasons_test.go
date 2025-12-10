package persephone

import (
	"context"
	"testing"
	"time"
)

func TestBasicSeasonalScaler(t *testing.T) {
	scaler := NewBasicSeasonalScaler()
	ctx := context.Background()

	// Test DefineSeason
	err := scaler.DefineSeason(ctx, SeasonSpring)
	if err != nil {
		t.Fatalf("DefineSeason failed: %v", err)
	}

	// Test ApplySeason
	err = scaler.ApplySeason(ctx, SeasonSpring.ID)
	if err != nil {
		t.Fatalf("ApplySeason failed: %v", err)
	}

	// Test CurrentSeason
	current, err := scaler.CurrentSeason(ctx)
	if err != nil {
		t.Fatalf("CurrentSeason failed: %v", err)
	}
	if current.ID != SeasonSpring.ID {
		t.Errorf("Expected season %s, got %s", SeasonSpring.ID, current.ID)
	}

	// Test Forecast
	forecast, err := scaler.Forecast(ctx, 0)
	if err != nil {
		t.Fatalf("Forecast failed: %v", err)
	}
	if forecast == nil {
		t.Error("Expected forecast, got nil")
	}
}

func TestRecommendCapacity_Prewarming(t *testing.T) {
	scaler := NewBasicSeasonalScaler()
	ctx := context.Background()

	// 1. Setup History with a strong daily pattern
	// Peak at 12:00 everyday.
	// Current simulated time will be 10:00.
	// We want to see pre-warming for 12:00.

	// Fix "Now" for the test logic (conceptually)
	// We construct history relative to the real Now()
	baseTime := time.Now()
	// Adjust baseTime so it looks like 10:00 locally if we were mocking time,
	// but since we use real time.Now() in RecommendCapacity logic (via Forecast),
	// we just need to ensure the pattern relative to Now() matches.

	// We want a peak at Now() + 2 hours.
	peakOffset := 2 * time.Hour

	history := []*UsageRecord{}

	// Add 7 days of history
	for day := 7; day > 0; day-- {
		// Normal low traffic
		startOfDay := baseTime.Add(time.Duration(-day) * 24 * time.Hour)

		// Peak happens at startOfDay + peakOffset
		for hour := 0; hour < 24; hour++ {
			ts := startOfDay.Add(time.Duration(hour) * time.Hour)

			active := 10 // Baseline

			// If this timeslot is within the peak window (peakOffset)
			// We check if the hour matches the peak hour relative to baseTime
			// Simple way: make the peak exactly at 2 hours from "now" in the cycle
			timeDiff := ts.Sub(startOfDay)
			if timeDiff >= peakOffset && timeDiff < peakOffset+2*time.Hour {
				active = 100 // Peak
			}

			history = append(history, &UsageRecord{
				Timestamp: ts,
				ActiveVMs: active,
			})
		}
	}
	scaler.Learn(ctx, history)

	// 2. Define a season with pre-warming
	season := &Season{
		ID:                "test-season",
		Name:              "Test",
		TargetUtilization: 1.0,
		MinNodes:          0,
		MaxNodes:          1000,
		Prewarming: PrewarmConfig{
			LeadTime: 2 * time.Hour,
		},
	}
	scaler.DefineSeason(ctx, season)
	scaler.ApplySeason(ctx, season.ID)

	// 3. Recommend Capacity
	// Current usage (at Now) should be low (10).
	// usage at Now + 2h should be high (100).
	// Pre-warming lead time is 2h.

	rec, err := scaler.RecommendCapacity(ctx, 1.0)
	if err != nil {
		t.Fatalf("RecommendCapacity failed: %v", err)
	}

	t.Logf("Current Nodes (approx): %d", rec.CurrentNodes)
	t.Logf("Recommended Nodes: %d", rec.RecommendedNodes)
	t.Logf("Reason: %s", rec.Reason)

	if rec.RecommendedNodes < 40 {
		t.Errorf("Expected recommended nodes > 40 due to pre-warming, got %d", rec.RecommendedNodes)
	}
}
