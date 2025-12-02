package charon

import (
	"fmt"
	"hash/crc32"
	"sort"
	"sync"
)

// ConsistentHashRing implements consistent hashing with virtual nodes.
// This provides better distribution and minimal disruption when adding/removing backends.
type ConsistentHashRing struct {
	// virtualNodes maps hash values to shore IDs
	virtualNodes map[uint32]string

	// sortedHashes contains all hash values in sorted order for binary search
	sortedHashes []uint32

	// replicas is the number of virtual nodes per physical shore
	replicas int

	mu sync.RWMutex
}

// NewConsistentHashRing creates a new consistent hash ring.
// replicas determines the number of virtual nodes per shore (typically 150-500).
// Higher replicas = better distribution but more memory.
func NewConsistentHashRing(replicas int) *ConsistentHashRing {
	if replicas <= 0 {
		replicas = 150 // Default virtual nodes
	}

	return &ConsistentHashRing{
		virtualNodes: make(map[uint32]string),
		sortedHashes: make([]uint32, 0),
		replicas:     replicas,
	}
}

// Add adds a shore to the hash ring.
func (r *ConsistentHashRing) Add(shoreID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create virtual nodes for this shore
	for i := 0; i < r.replicas; i++ {
		// Hash the shore ID with replica number to create virtual nodes
		virtualKey := fmt.Sprintf("%s#%d", shoreID, i)
		hash := r.hashKey(virtualKey)

		r.virtualNodes[hash] = shoreID
		r.sortedHashes = append(r.sortedHashes, hash)
	}

	// Keep hashes sorted for efficient lookup
	sort.Slice(r.sortedHashes, func(i, j int) bool {
		return r.sortedHashes[i] < r.sortedHashes[j]
	})
}

// Remove removes a shore from the hash ring.
func (r *ConsistentHashRing) Remove(shoreID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove all virtual nodes for this shore
	for i := 0; i < r.replicas; i++ {
		virtualKey := fmt.Sprintf("%s#%d", shoreID, i)
		hash := r.hashKey(virtualKey)
		delete(r.virtualNodes, hash)
	}

	// Rebuild sorted hashes
	r.sortedHashes = make([]uint32, 0, len(r.virtualNodes))
	for hash := range r.virtualNodes {
		r.sortedHashes = append(r.sortedHashes, hash)
	}

	sort.Slice(r.sortedHashes, func(i, j int) bool {
		return r.sortedHashes[i] < r.sortedHashes[j]
	})
}

// Get returns the shore ID for the given key using consistent hashing.
// Returns empty string if the ring is empty.
func (r *ConsistentHashRing) Get(key string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.sortedHashes) == 0 {
		return ""
	}

	hash := r.hashKey(key)

	// Binary search to find the first hash >= our key's hash
	idx := sort.Search(len(r.sortedHashes), func(i int) bool {
		return r.sortedHashes[i] >= hash
	})

	// Wrap around to the beginning if we've gone past the end
	if idx >= len(r.sortedHashes) {
		idx = 0
	}

	return r.virtualNodes[r.sortedHashes[idx]]
}

// GetN returns up to N shores for the given key, in priority order.
// This is useful for fallback when the primary shore is unhealthy.
func (r *ConsistentHashRing) GetN(key string, n int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.sortedHashes) == 0 || n <= 0 {
		return nil
	}

	hash := r.hashKey(key)

	// Binary search to find starting position
	idx := sort.Search(len(r.sortedHashes), func(i int) bool {
		return r.sortedHashes[i] >= hash
	})

	if idx >= len(r.sortedHashes) {
		idx = 0
	}

	// Collect unique shore IDs
	seen := make(map[string]bool)
	result := make([]string, 0, n)

	// Walk the ring and collect unique shores
	for i := 0; i < len(r.sortedHashes) && len(result) < n; i++ {
		pos := (idx + i) % len(r.sortedHashes)
		shoreID := r.virtualNodes[r.sortedHashes[pos]]

		if !seen[shoreID] {
			seen[shoreID] = true
			result = append(result, shoreID)
		}
	}

	return result
}

// Size returns the number of physical shores in the ring.
func (r *ConsistentHashRing) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Count unique shore IDs
	seen := make(map[string]bool)
	for _, shoreID := range r.virtualNodes {
		seen[shoreID] = true
	}

	return len(seen)
}

// hashKey computes a hash for the given key using CRC32.
// CRC32 is fast and provides good distribution for consistent hashing.
func (r *ConsistentHashRing) hashKey(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}
