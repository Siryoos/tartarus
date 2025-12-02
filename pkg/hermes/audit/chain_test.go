package audit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainManager_ComputeHash(t *testing.T) {
	cm := NewChainManager([]byte("secret"))
	event := &Event{
		ID:        "1",
		Timestamp: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Action:    ActionLogin,
		Result:    ResultSuccess,
	}

	hash1, err := cm.ComputeHash(event)
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)

	// Same event should produce same hash
	hash2, err := cm.ComputeHash(event)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)

	// Different event should produce different hash
	event.Result = ResultDenied
	hash3, err := cm.ComputeHash(event)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

func TestChainManager_VerifyChain(t *testing.T) {
	cm := NewChainManager([]byte("secret"))

	event1 := Event{
		ID:        "1",
		Timestamp: time.Now(),
		Action:    ActionLogin,
	}
	hash1, err := cm.ComputeHash(&event1)
	require.NoError(t, err)
	event1.Hash = hash1

	event2 := Event{
		ID:           "2",
		Timestamp:    time.Now(),
		Action:       ActionRead,
		PreviousHash: hash1,
	}
	hash2, err := cm.ComputeHash(&event2)
	require.NoError(t, err)
	event2.Hash = hash2

	events := []Event{event1, event2}

	// Valid chain
	err = cm.VerifyChain(events)
	assert.NoError(t, err)

	// Broken chain (tampered hash)
	events[0].Hash = "tampered"
	err = cm.VerifyChain(events)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")

	// Broken chain (tampered previous hash link)
	events[0].Hash = hash1 // restore
	events[1].PreviousHash = "tampered"
	// Note: VerifyChain checks if event.Hash matches ComputeHash(event).
	// If we change PreviousHash, ComputeHash will change, so event.Hash won't match.
	// We need to update event.Hash to match the new PreviousHash to test the link check specifically?
	// Actually, if we just change PreviousHash, the first check (Hash mismatch) will fail.
	// To test the second check (link mismatch), we need a valid hash for the tampered PreviousHash.

	tamperedHash2, _ := cm.ComputeHash(&events[1])
	events[1].Hash = tamperedHash2

	err = cm.VerifyChain(events)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chain broken")
}
