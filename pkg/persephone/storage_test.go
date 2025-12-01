package persephone

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalHistoryStore(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "persephone-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store, err := NewLocalHistoryStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Test Save and Load
	now := time.Now().Truncate(time.Second)
	records := []*UsageRecord{
		{Timestamp: now, ActiveVMs: 10, QueueDepth: 2},
		{Timestamp: now.Add(time.Minute), ActiveVMs: 15, QueueDepth: 5},
		{Timestamp: now.Add(2 * time.Minute), ActiveVMs: 20, QueueDepth: 0},
	}

	err = store.Save(ctx, records)
	require.NoError(t, err)

	// Load all records
	loaded, err := store.Load(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 3, len(loaded))
	assert.Equal(t, 10, loaded[0].ActiveVMs)
	assert.Equal(t, 20, loaded[2].ActiveVMs)

	// Test QueryRecent
	recent, err := store.QueryRecent(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, len(recent))
	assert.Equal(t, 15, recent[0].ActiveVMs)
	assert.Equal(t, 20, recent[1].ActiveVMs)

	// Test Prune
	oldRecord := []*UsageRecord{
		{Timestamp: now.AddDate(0, 0, -100), ActiveVMs: 5},
	}
	err = store.Save(ctx, oldRecord)
	require.NoError(t, err)

	// Verify we have 4 records now
	all, err := store.Load(ctx, now.AddDate(0, 0, -200), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 4, len(all))

	// Prune records older than 90 days
	err = store.Prune(ctx, 90)
	require.NoError(t, err)

	// Verify old record was removed
	all, err = store.Load(ctx, now.AddDate(0, 0, -200), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 3, len(all))
}

func TestLocalHistoryStore_TimeRangeFilter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "persephone-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store, err := NewLocalHistoryStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Save records over a 24-hour period
	records := []*UsageRecord{
		{Timestamp: now.Add(-12 * time.Hour), ActiveVMs: 5},
		{Timestamp: now.Add(-6 * time.Hour), ActiveVMs: 10},
		{Timestamp: now, ActiveVMs: 15},
		{Timestamp: now.Add(6 * time.Hour), ActiveVMs: 20},
	}
	err = store.Save(ctx, records)
	require.NoError(t, err)

	// Load only records from last 8 hours
	filtered, err := store.Load(ctx, now.Add(-8*time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, 10, filtered[0].ActiveVMs)
	assert.Equal(t, 15, filtered[1].ActiveVMs)
}

func TestLocalHistoryStore_EmptyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "persephone-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store, err := NewLocalHistoryStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Query from empty store should return empty slice
	recent, err := store.QueryRecent(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, len(recent))

	loaded, err := store.Load(ctx, time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 0, len(loaded))
}

func TestLocalHistoryStore_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "persephone-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create store and save data
	store1, err := NewLocalHistoryStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	records := []*UsageRecord{
		{Timestamp: now, ActiveVMs: 10},
	}
	err = store1.Save(ctx, records)
	require.NoError(t, err)
	store1.Close()

	// Create new store instance and verify data persisted
	store2, err := NewLocalHistoryStore(tmpDir)
	require.NoError(t, err)
	defer store2.Close()

	loaded, err := store2.Load(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, len(loaded))
	assert.Equal(t, 10, loaded[0].ActiveVMs)
}

// Note: Redis tests would require testcontainers or mock
// Skipping Redis tests for now to avoid external dependencies in unit tests
// Integration tests will cover Redis functionality
