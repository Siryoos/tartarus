package persephone

import (
	"context"
	"testing"
	"time"
)

// MockCollector for testing
type MockCollector struct {
	records []*UsageRecord
	err     error
}

func (m *MockCollector) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]*UsageRecord, error) {
	return m.records, m.err
}

// MockStore for testing
type MockStore struct {
	saved       []*UsageRecord
	loadRecords []*UsageRecord
}

func (m *MockStore) Save(ctx context.Context, records []*UsageRecord) error {
	m.saved = append(m.saved, records...)
	return nil
}

func (m *MockStore) Load(ctx context.Context, start, end time.Time) ([]*UsageRecord, error) {
	return m.loadRecords, nil
}

func (m *MockStore) QueryRecent(ctx context.Context, count int) ([]*UsageRecord, error) {
	return nil, nil // Not needed for Ingestor test
}

func (m *MockStore) Prune(ctx context.Context, retentionDays int) error {
	return nil
}

func (m *MockStore) Close() error {
	return nil
}

func TestIngestor_Ingest(t *testing.T) {
	// Setup
	mockCollector := &MockCollector{
		records: []*UsageRecord{
			{Timestamp: time.Now(), ActiveVMs: 10},
			{Timestamp: time.Now().Add(time.Minute), ActiveVMs: 12},
		},
	}
	mockStore := &MockStore{}

	ingestor, err := NewIngestor(IngestorConfig{
		Collector: mockCollector,
		Store:     mockStore,
		Interval:  time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to create ingestor: %v", err)
	}

	// Test Ingest
	ctx := context.Background()
	if err := ingestor.ingest(ctx); err != nil {
		t.Errorf("Ingest failed: %v", err)
	}

	// Verify
	if len(mockStore.saved) != 2 {
		t.Errorf("Expected 2 saved records, got %d", len(mockStore.saved))
	}
	if mockStore.saved[0].ActiveVMs != 10 {
		t.Errorf("Expected first record ActiveVMs=10, got %d", mockStore.saved[0].ActiveVMs)
	}
}
