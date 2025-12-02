package charon

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsistentHashRing_Basic(t *testing.T) {
	ring := NewConsistentHashRing(150)

	// Add some shores
	ring.Add("shore-1")
	ring.Add("shore-2")
	ring.Add("shore-3")

	assert.Equal(t, 3, ring.Size())

	// Same key should always map to the same shore
	key := "user-123"
	shore1 := ring.Get(key)
	shore2 := ring.Get(key)
	assert.Equal(t, shore1, shore2)
	assert.NotEmpty(t, shore1)
}

func TestConsistentHashRing_Distribution(t *testing.T) {
	ring := NewConsistentHashRing(150)

	// Add shores
	shores := []string{"shore-1", "shore-2", "shore-3", "shore-4", "shore-5"}
	for _, shore := range shores {
		ring.Add(shore)
	}

	// Generate many keys and track distribution
	distribution := make(map[string]int)
	numKeys := 10000

	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key-%d", i)
		shore := ring.Get(key)
		distribution[shore]++
	}

	// Each shore should get roughly 20% of keys (10000 / 5 = 2000)
	// Allow for some variance (15-25%)
	expectedPerShore := numKeys / len(shores)
	for _, shore := range shores {
		count := distribution[shore]
		percentage := float64(count) / float64(numKeys) * 100
		t.Logf("Shore %s: %d keys (%.2f%%)", shore, count, percentage)

		assert.Greater(t, count, int(float64(expectedPerShore)*0.7), "Shore %s got too few keys", shore)
		assert.Less(t, count, int(float64(expectedPerShore)*1.3), "Shore %s got too many keys", shore)
	}
}

func TestConsistentHashRing_AddRemove(t *testing.T) {
	ring := NewConsistentHashRing(150)

	// Add initial shores
	ring.Add("shore-1")
	ring.Add("shore-2")
	ring.Add("shore-3")

	// Track where keys go
	numKeys := 1000
	initialMapping := make(map[string]string)

	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key-%d", i)
		initialMapping[key] = ring.Get(key)
	}

	// Add a new shore
	ring.Add("shore-4")

	// Check how many keys moved
	moved := 0
	for key, initialShore := range initialMapping {
		newShore := ring.Get(key)
		if newShore != initialShore {
			moved++
		}
	}

	// With consistent hashing, only ~1/4 of keys should move
	// (new shore takes 1/4 of the load from existing 3 shores)
	movePercentage := float64(moved) / float64(numKeys) * 100
	t.Logf("Keys moved after adding shore-4: %d/%d (%.2f%%)", moved, numKeys, movePercentage)

	// Should be roughly 25% (+/- some variance)
	assert.Greater(t, movePercentage, 10.0, "Too few keys moved")
	assert.Less(t, movePercentage, 40.0, "Too many keys moved")

	// Remove a shore
	ring.Remove("shore-2")

	// Track new mapping
	finalMapping := make(map[string]string)
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key-%d", i)
		finalMapping[key] = ring.Get(key)
	}

	// Keys that were on shore-2 should move, others should stay
	movedAfterRemove := 0
	for key, newShore := range finalMapping {
		oldShore := initialMapping[key]
		if oldShore == "shore-2" {
			// This key must have moved
			assert.NotEqual(t, "shore-2", newShore)
		} else {
			// This key might have moved (if it was reassigned to shore-4)
			if newShore != oldShore {
				movedAfterRemove++
			}
		}
	}

	t.Logf("Keys moved after removing shore-2: %d", movedAfterRemove)
}

func TestConsistentHashRing_GetN(t *testing.T) {
	ring := NewConsistentHashRing(150)

	shores := []string{"shore-1", "shore-2", "shore-3", "shore-4"}
	for _, shore := range shores {
		ring.Add(shore)
	}

	key := "user-456"

	// Get primary shore
	primary := ring.Get(key)
	assert.NotEmpty(t, primary)

	// Get top 3 shores for this key (primary + 2 fallbacks)
	top3 := ring.GetN(key, 3)
	require.Len(t, top3, 3)

	// Primary should be first
	assert.Equal(t, primary, top3[0])

	// All should be unique
	seen := make(map[string]bool)
	for _, shore := range top3 {
		assert.False(t, seen[shore], "Duplicate shore in GetN result")
		seen[shore] = true
	}
}

func TestConsistentHashRing_EmptyRing(t *testing.T) {
	ring := NewConsistentHashRing(150)

	assert.Equal(t, 0, ring.Size())
	assert.Empty(t, ring.Get("any-key"))
	assert.Nil(t, ring.GetN("any-key", 3))
}

func TestConsistentHashRing_StickySession(t *testing.T) {
	ring := NewConsistentHashRing(150)

	ring.Add("shore-1")
	ring.Add("shore-2")
	ring.Add("shore-3")

	// Simulate a user session - same session ID should always go to same shore
	sessionID := "session-abc123"

	// Make 100 requests with the same session ID
	shores := make(map[string]int)
	for i := 0; i < 100; i++ {
		shore := ring.Get(sessionID)
		shores[shore]++
	}

	// All 100 requests should go to the same shore
	assert.Equal(t, 1, len(shores), "Session should stick to single shore")
	for shore, count := range shores {
		t.Logf("Session %s mapped to %s (%d requests)", sessionID, shore, count)
		assert.Equal(t, 100, count)
	}
}

func BenchmarkConsistentHashRing_Get(b *testing.B) {
	ring := NewConsistentHashRing(150)

	for i := 0; i < 10; i++ {
		ring.Add(fmt.Sprintf("shore-%d", i))
	}

	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring.Get(keys[i%len(keys)])
	}
}

func BenchmarkConsistentHashRing_Add(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring := NewConsistentHashRing(150)
		for j := 0; j < 10; j++ {
			ring.Add(fmt.Sprintf("shore-%d", j))
		}
	}
}
