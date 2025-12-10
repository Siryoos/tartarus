package persephone

import (
	"context"
	"testing"
	"time"
)

func TestBudgetTracker_RecordSpend(t *testing.T) {
	tracker := NewBudgetTracker(nil)
	ctx := context.Background()

	// Record some spending
	tracker.RecordSpend(ctx, 10.0)
	tracker.RecordSpend(ctx, 5.0)

	daily := tracker.GetDailySpend(ctx)
	if daily != 15.0 {
		t.Errorf("Expected daily spend 15.0, got %f", daily)
	}

	monthly := tracker.GetMonthlySpend(ctx)
	if monthly != 15.0 {
		t.Errorf("Expected monthly spend 15.0, got %f", monthly)
	}
}

func TestBudgetTracker_CheckBudget_UnderBudget(t *testing.T) {
	tracker := NewBudgetTracker(nil)
	ctx := context.Background()

	tracker.RecordSpend(ctx, 50.0)

	config := BudgetConfig{
		DailyLimit:     100.0,
		MonthlyLimit:   1000.0,
		AlertThreshold: 0.8,
		HardCap:        true,
	}

	status := tracker.CheckBudget(ctx, config)

	if status.OverBudget {
		t.Error("Should not be over budget")
	}
	if status.AlertTriggered {
		t.Error("Alert should not be triggered at 50%")
	}
	if status.DailyRemaining != 50.0 {
		t.Errorf("Expected daily remaining 50.0, got %f", status.DailyRemaining)
	}
}

func TestBudgetTracker_CheckBudget_AlertThreshold(t *testing.T) {
	tracker := NewBudgetTracker(nil)
	ctx := context.Background()

	// Spend 85% of daily budget
	tracker.RecordSpend(ctx, 85.0)

	config := BudgetConfig{
		DailyLimit:     100.0,
		MonthlyLimit:   1000.0,
		AlertThreshold: 0.8, // Alert at 80%
		HardCap:        true,
	}

	status := tracker.CheckBudget(ctx, config)

	if status.OverBudget {
		t.Error("Should not be over budget")
	}
	if !status.AlertTriggered {
		t.Error("Alert should be triggered at 85%")
	}
}

func TestBudgetTracker_CheckBudget_OverBudget(t *testing.T) {
	tracker := NewBudgetTracker(nil)
	ctx := context.Background()

	// Spend over daily budget
	tracker.RecordSpend(ctx, 150.0)

	config := BudgetConfig{
		DailyLimit:     100.0,
		MonthlyLimit:   1000.0,
		AlertThreshold: 0.8,
		HardCap:        true,
	}

	status := tracker.CheckBudget(ctx, config)

	if !status.OverBudget {
		t.Error("Should be over budget")
	}
	if !status.AlertTriggered {
		t.Error("Alert should be triggered when over budget")
	}
}

func TestBudgetTracker_CalculateMaxAffordableNodes(t *testing.T) {
	tracker := NewBudgetTracker(nil)
	ctx := context.Background()

	// Spend $50, leaving $50 remaining in daily budget
	tracker.RecordSpend(ctx, 50.0)

	config := BudgetConfig{
		DailyLimit:      100.0,
		MonthlyLimit:    1000.0,
		CostPerNodeHour: 10.0,
		HardCap:         true,
	}

	// With $50 remaining and $10/node/hour, can afford 5 nodes for 1 hour
	maxNodes := tracker.CalculateMaxAffordableNodes(ctx, config, 1.0)
	if maxNodes != 5 {
		t.Errorf("Expected 5 affordable nodes, got %d", maxNodes)
	}

	// For 2 hours, can only afford 2 nodes
	maxNodes = tracker.CalculateMaxAffordableNodes(ctx, config, 2.0)
	if maxNodes != 2 {
		t.Errorf("Expected 2 affordable nodes for 2 hours, got %d", maxNodes)
	}
}

func TestBudgetTracker_PruneOldRecords(t *testing.T) {
	tracker := NewBudgetTracker(nil)
	ctx := context.Background()

	// Mock time to test pruning
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	// Record spending on different days
	tracker.now = func() time.Time { return baseTime.AddDate(0, 0, -10) }
	tracker.RecordSpend(ctx, 100.0)

	tracker.now = func() time.Time { return baseTime.AddDate(0, 0, -5) }
	tracker.RecordSpend(ctx, 50.0)

	tracker.now = func() time.Time { return baseTime }
	tracker.RecordSpend(ctx, 25.0)

	// Prune records older than 7 days
	tracker.PruneOldRecords(ctx, 7)

	// Daily spend should only include records from within last 7 days
	daily := tracker.GetDailySpend(ctx)
	if daily != 25.0 {
		t.Errorf("Expected 25.0 daily spend after prune, got %f", daily)
	}
}

func TestBudgetEnforcer_RecommendCapacityWithBudget(t *testing.T) {
	// Create a mock scaler
	scaler := NewBasicSeasonalScaler()
	ctx := context.Background()

	// Add some history to generate recommendations
	history := []*UsageRecord{
		{Timestamp: time.Now(), ActiveVMs: 50},
	}
	scaler.Learn(ctx, history)

	// Create tracker and enforcer
	tracker := NewBudgetTracker(nil)
	enforcer := NewBudgetEnforcer(scaler, tracker)

	// Spend most of budget
	tracker.RecordSpend(ctx, 90.0)

	budget := BudgetConfig{
		DailyLimit:      100.0,
		MonthlyLimit:    1000.0,
		CostPerNodeHour: 10.0,
		HardCap:         true,
	}

	// Get recommendation with budget enforcement
	rec, err := enforcer.RecommendCapacityWithBudget(ctx, 0.8, budget)
	if err != nil {
		t.Fatalf("RecommendCapacityWithBudget failed: %v", err)
	}

	// With only $10 remaining and $10/node, should be capped at 1 node
	if rec.RecommendedNodes > 1 {
		t.Errorf("Expected recommendation capped at 1 node, got %d", rec.RecommendedNodes)
	}
}

func TestBudgetEnforcer_NoHardCap(t *testing.T) {
	scaler := NewBasicSeasonalScaler()
	ctx := context.Background()

	history := []*UsageRecord{
		{Timestamp: time.Now(), ActiveVMs: 100},
	}
	scaler.Learn(ctx, history)

	tracker := NewBudgetTracker(nil)
	enforcer := NewBudgetEnforcer(scaler, tracker)

	// Spend all budget
	tracker.RecordSpend(ctx, 150.0)

	budget := BudgetConfig{
		DailyLimit:      100.0,
		MonthlyLimit:    1000.0,
		CostPerNodeHour: 10.0,
		HardCap:         false, // No hard cap
	}

	// Should return original recommendation without enforcement
	rec, err := enforcer.RecommendCapacityWithBudget(ctx, 0.8, budget)
	if err != nil {
		t.Fatalf("RecommendCapacityWithBudget failed: %v", err)
	}

	// Without hard cap, should still recommend scaling
	if rec.RecommendedNodes == 0 {
		t.Error("Expected non-zero recommendation without hard cap")
	}
}
