package evaluator

import (
	"context"
	"testing"
	"time"

	"github.com/tartarus-sandbox/tartarus/pkg/persephone"
)

// MockHistoryStore for testing
type MockHistoryStore struct {
	records []*persephone.UsageRecord
}

func (m *MockHistoryStore) Save(ctx context.Context, records []*persephone.UsageRecord) error {
	m.records = append(m.records, records...)
	return nil
}

func (m *MockHistoryStore) Load(ctx context.Context, start, end time.Time) ([]*persephone.UsageRecord, error) {
	var result []*persephone.UsageRecord
	for _, r := range m.records {
		if (r.Timestamp.Equal(start) || r.Timestamp.After(start)) && r.Timestamp.Before(end) {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *MockHistoryStore) QueryRecent(ctx context.Context, count int) ([]*persephone.UsageRecord, error) {
	return nil, nil
}
func (m *MockHistoryStore) Prune(ctx context.Context, retentionDays int) error { return nil }
func (m *MockHistoryStore) Close() error                                       { return nil }

func TestCalculateMetrics(t *testing.T) {
	preds := []float64{10, 20, 30}
	actuals := []float64{12, 18, 33}
	lower := []float64{8, 15, 25}
	upper := []float64{15, 25, 35}

	metrics := CalculateMetrics(preds, actuals, lower, upper)

	if metrics.MAE == 0 {
		t.Error("MAE should not be 0")
	}
	if metrics.Coverage < 100 {
		t.Error("Expected 100% coverage")
	}
}

func TestBacktester(t *testing.T) {
	store := &MockHistoryStore{}
	now := time.Now().Truncate(time.Hour)

	// Generate synthetic data: Sine wave pattern
	for i := 0; i < 100; i++ {
		ts := now.Add(time.Duration(i) * time.Hour)
		val := 10.0 + 5.0*float64(i%24)/24.0 // Simple pattern
		store.records = append(store.records, &persephone.UsageRecord{
			Timestamp: ts,
			ActiveVMs: int(val),
		})
	}

	tester := NewBacktester(store)
	start := now.Add(48 * time.Hour)
	end := now.Add(72 * time.Hour)

	report, err := tester.Run(context.Background(), start, end, 24*time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("Backtest failed: %v", err)
	}

	if len(report.Predictions) == 0 {
		t.Error("No predictions generated")
	}
}
