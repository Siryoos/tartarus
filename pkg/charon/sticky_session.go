package charon

import (
	"sync"
	"time"
)

// stickyEntry is a single pinned affinity entry.
type stickyEntry struct {
	shoreID   string
	pinnedAt  time.Time
	draining  bool  // set when the target shore is deregistered
	drainedAt time.Time
}

// StickySessionTable maintains session-key → shore-ID affinity across shore
// deregistrations. When a shore is removed, its entries are marked draining
// rather than deleted immediately. A background sweeper evicts draining entries
// after DrainTimeout has elapsed, giving in-flight sessions time to complete.
type StickySessionTable struct {
	entries      sync.Map      // map[string]*stickyEntry
	DrainTimeout time.Duration // how long to keep draining entries (default 5 min)

	stopCh chan struct{}
	doneCh chan struct{}
}

// NewStickySessionTable creates and starts a StickySessionTable.
// drainTimeout is the grace period after shore deregistration before sessions
// are evicted. Pass 0 to use the default of 5 minutes.
func NewStickySessionTable(drainTimeout time.Duration) *StickySessionTable {
	if drainTimeout <= 0 {
		drainTimeout = 5 * time.Minute
	}
	t := &StickySessionTable{
		DrainTimeout: drainTimeout,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
	go t.sweepLoop()
	return t
}

// Pin records (or refreshes) a session-key → shore-ID affinity.
// If the entry already exists and is not draining, it is left untouched.
// If it was draining (the old shore is being removed), it is re-pinned to the
// new shore.
func (t *StickySessionTable) Pin(key, shoreID string) {
	entry := &stickyEntry{
		shoreID:  shoreID,
		pinnedAt: time.Now(),
	}
	// Store unconditionally so new shoreID overwrites a draining entry.
	t.entries.Store(key, entry)
}

// Get returns the pinned shore ID for key if the affinity is still active
// (not yet evicted). The second return value is false when no valid affinity
// exists. Draining entries are still returned so callers can detect the drain
// state and decide whether to re-pin.
func (t *StickySessionTable) Get(key string) (shoreID string, draining bool, ok bool) {
	raw, exists := t.entries.Load(key)
	if !exists {
		return "", false, false
	}
	e := raw.(*stickyEntry)
	return e.shoreID, e.draining, true
}

// Drain marks all entries pinned to shoreID as draining. Draining entries
// are still returned by Get until DrainTimeout expires, giving in-flight
// requests a chance to finish on their sticky shore before being re-hashed.
func (t *StickySessionTable) Drain(shoreID string) {
	now := time.Now()
	t.entries.Range(func(k, v interface{}) bool {
		e := v.(*stickyEntry)
		if e.shoreID == shoreID && !e.draining {
			e.draining = true
			e.drainedAt = now
		}
		return true
	})
}

// Evict hard-removes all entries pinned to shoreID without waiting for the
// drain timeout. Use when a shore is permanently removed and no grace period
// is desired.
func (t *StickySessionTable) Evict(shoreID string) {
	t.entries.Range(func(k, v interface{}) bool {
		e := v.(*stickyEntry)
		if e.shoreID == shoreID {
			t.entries.Delete(k)
		}
		return true
	})
}

// Close stops the background sweeper. Safe to call multiple times.
func (t *StickySessionTable) Close() {
	select {
	case <-t.stopCh:
		// already closed
	default:
		close(t.stopCh)
		<-t.doneCh
	}
}

// sweepLoop runs in the background and evicts drained entries whose grace
// period has expired. It also prunes very old pinned entries (> 2× DrainTimeout)
// to prevent unbounded growth.
func (t *StickySessionTable) sweepLoop() {
	defer close(t.doneCh)

	ticker := time.NewTicker(t.DrainTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.sweep()
		case <-t.stopCh:
			return
		}
	}
}

func (t *StickySessionTable) sweep() {
	now := time.Now()
	staleThreshold := now.Add(-2 * t.DrainTimeout)

	t.entries.Range(func(k, v interface{}) bool {
		e := v.(*stickyEntry)
		switch {
		case e.draining && now.After(e.drainedAt.Add(t.DrainTimeout)):
			// Grace period expired — evict.
			t.entries.Delete(k)
		case !e.draining && e.pinnedAt.Before(staleThreshold):
			// Very old unpinned entry — evict to prevent memory growth.
			t.entries.Delete(k)
		}
		return true
	})
}
