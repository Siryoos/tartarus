package persephone

import (
	"context"
	"testing"
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
