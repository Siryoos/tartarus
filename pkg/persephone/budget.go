package persephone

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/hermes"
)

// BudgetConfig defines spending limits for a season
type BudgetConfig struct {
	// DailyLimit is the maximum spending per day in dollars
	DailyLimit float64
	// MonthlyLimit is the maximum spending per month in dollars
	MonthlyLimit float64
	// AlertThreshold is the percentage (0.0-1.0) of budget at which to trigger alerts
	AlertThreshold float64
	// HardCap if true strictly prevents scaling beyond budget; if false, only alerts
	HardCap bool
	// CostPerNodeHour is the cost per node per hour in dollars
	CostPerNodeHour float64
}

// BudgetStatus represents current budget consumption state
type BudgetStatus struct {
	DailySpent       float64
	DailyRemaining   float64
	DailyLimit       float64
	MonthlySpent     float64
	MonthlyRemaining float64
	MonthlyLimit     float64
	AlertTriggered   bool
	OverBudget       bool
}

// BudgetTracker monitors and enforces budget constraints
type BudgetTracker struct {
	mu sync.RWMutex

	// Spending records keyed by date (YYYY-MM-DD) and month (YYYY-MM)
	dailySpending   map[string]float64
	monthlySpending map[string]float64

	// Metrics for alerting
	metrics hermes.Metrics

	// Time source for testing
	now func() time.Time
}

// NewBudgetTracker creates a new budget tracker
func NewBudgetTracker(metrics hermes.Metrics) *BudgetTracker {
	return &BudgetTracker{
		dailySpending:   make(map[string]float64),
		monthlySpending: make(map[string]float64),
		metrics:         metrics,
		now:             time.Now,
	}
}

// RecordSpend records spending for the current time period
func (b *BudgetTracker) RecordSpend(ctx context.Context, amount float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	dayKey := now.Format("2006-01-02")
	monthKey := now.Format("2006-01")

	b.dailySpending[dayKey] += amount
	b.monthlySpending[monthKey] += amount
}

// GetDailySpend returns the spending for the current day
func (b *BudgetTracker) GetDailySpend(ctx context.Context) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	dayKey := b.now().Format("2006-01-02")
	return b.dailySpending[dayKey]
}

// GetMonthlySpend returns the spending for the current month
func (b *BudgetTracker) GetMonthlySpend(ctx context.Context) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	monthKey := b.now().Format("2006-01")
	return b.monthlySpending[monthKey]
}

// CheckBudget evaluates budget status against the given configuration
func (b *BudgetTracker) CheckBudget(ctx context.Context, config BudgetConfig) *BudgetStatus {
	dailySpent := b.GetDailySpend(ctx)
	monthlySpent := b.GetMonthlySpend(ctx)

	status := &BudgetStatus{
		DailySpent:       dailySpent,
		DailyLimit:       config.DailyLimit,
		DailyRemaining:   config.DailyLimit - dailySpent,
		MonthlySpent:     monthlySpent,
		MonthlyLimit:     config.MonthlyLimit,
		MonthlyRemaining: config.MonthlyLimit - monthlySpent,
	}

	// Check if over budget
	if config.DailyLimit > 0 && dailySpent >= config.DailyLimit {
		status.OverBudget = true
	}
	if config.MonthlyLimit > 0 && monthlySpent >= config.MonthlyLimit {
		status.OverBudget = true
	}

	// Check alert threshold
	if config.AlertThreshold > 0 {
		dailyThreshold := config.DailyLimit * config.AlertThreshold
		monthlyThreshold := config.MonthlyLimit * config.AlertThreshold

		if (config.DailyLimit > 0 && dailySpent >= dailyThreshold) ||
			(config.MonthlyLimit > 0 && monthlySpent >= monthlyThreshold) {
			status.AlertTriggered = true

			// Emit metric for alerting
			if b.metrics != nil {
				b.metrics.IncCounter("persephone_budget_alert_total", 1,
					hermes.Label{Key: "type", Value: "threshold_exceeded"})
			}
		}
	}

	return status
}

// CalculateMaxAffordableNodes calculates the maximum nodes that can be run within budget
func (b *BudgetTracker) CalculateMaxAffordableNodes(ctx context.Context, config BudgetConfig, hours float64) int {
	status := b.CheckBudget(ctx, config)

	if config.CostPerNodeHour <= 0 || hours <= 0 {
		return 0
	}

	// Use the most restrictive remaining budget
	remaining := status.DailyRemaining
	if config.MonthlyLimit > 0 && status.MonthlyRemaining < remaining {
		remaining = status.MonthlyRemaining
	}

	if remaining <= 0 {
		return 0
	}

	// Calculate max nodes: remaining / (cost per node * hours)
	costPerNodePeriod := config.CostPerNodeHour * hours
	maxNodes := int(remaining / costPerNodePeriod)

	return maxNodes
}

// RemainingDailyBudget returns the remaining daily budget
func (b *BudgetTracker) RemainingDailyBudget(ctx context.Context, config BudgetConfig) float64 {
	return config.DailyLimit - b.GetDailySpend(ctx)
}

// RemainingMonthlyBudget returns the remaining monthly budget
func (b *BudgetTracker) RemainingMonthlyBudget(ctx context.Context, config BudgetConfig) float64 {
	return config.MonthlyLimit - b.GetMonthlySpend(ctx)
}

// PruneOldRecords removes spending records older than the retention period
func (b *BudgetTracker) PruneOldRecords(ctx context.Context, retentionDays int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := b.now().AddDate(0, 0, -retentionDays)
	cutoffDay := cutoff.Format("2006-01-02")
	cutoffMonth := cutoff.Format("2006-01")

	// Prune daily records
	for key := range b.dailySpending {
		if key < cutoffDay {
			delete(b.dailySpending, key)
		}
	}

	// Prune monthly records
	for key := range b.monthlySpending {
		if key < cutoffMonth {
			delete(b.monthlySpending, key)
		}
	}
}

// BudgetEnforcer wraps a SeasonalScaler to enforce budget constraints
type BudgetEnforcer struct {
	scaler  SeasonalScaler
	tracker *BudgetTracker
}

// NewBudgetEnforcer creates a budget-aware scaler wrapper
func NewBudgetEnforcer(scaler SeasonalScaler, tracker *BudgetTracker) *BudgetEnforcer {
	return &BudgetEnforcer{
		scaler:  scaler,
		tracker: tracker,
	}
}

// RecommendCapacityWithBudget returns capacity recommendation constrained by budget
func (e *BudgetEnforcer) RecommendCapacityWithBudget(
	ctx context.Context,
	targetUtil float64,
	budget BudgetConfig,
) (*CapacityRecommendation, error) {
	// Get base recommendation from scaler
	rec, err := e.scaler.RecommendCapacity(ctx, targetUtil)
	if err != nil {
		return nil, err
	}

	// If no hard cap, return original recommendation
	if !budget.HardCap {
		return rec, nil
	}

	// Check budget and constrain if needed
	status := e.tracker.CheckBudget(ctx, budget)
	if status.OverBudget {
		// Can't scale up at all
		rec.RecommendedNodes = rec.CurrentNodes
		rec.Reason = fmt.Sprintf("%s (blocked by budget: daily $%.2f/$%.2f, monthly $%.2f/$%.2f)",
			rec.Reason, status.DailySpent, status.DailyLimit, status.MonthlySpent, status.MonthlyLimit)
		return rec, nil
	}

	// Calculate max affordable nodes (assuming 1 hour periods)
	maxAffordable := e.tracker.CalculateMaxAffordableNodes(ctx, budget, 1.0)
	if maxAffordable > 0 && rec.RecommendedNodes > maxAffordable {
		originalRec := rec.RecommendedNodes
		rec.RecommendedNodes = maxAffordable
		rec.Reason = fmt.Sprintf("%s (reduced from %d to %d by budget)",
			rec.Reason, originalRec, maxAffordable)
	}

	return rec, nil
}
