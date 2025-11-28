package charon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter controls request flow to prevent overload.
type RateLimiter interface {
	// Allow checks if a request should be allowed for the given key
	Allow(ctx context.Context, key string) error

	// Close releases resources
	Close() error
}

// KeyFunc extracts the rate limit key from a request.
type KeyFunc func(ctx context.Context) string

// TokenBucketLimiter implements rate limiting using token bucket algorithm.
type TokenBucketLimiter struct {
	requestsPerSecond int
	burst             int
	keyFunc           KeyFunc

	// Per-key limiters with last access time
	limiters map[string]*limiterEntry
	mu       sync.RWMutex

	// Cleanup
	cleanupInterval time.Duration
	cleanupStop     chan struct{}
	cleanupDone     chan struct{}
}

type limiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// NewTokenBucketLimiter creates a new token bucket rate limiter.
func NewTokenBucketLimiter(requestsPerSecond, burst int, keyFunc KeyFunc) *TokenBucketLimiter {
	limiter := &TokenBucketLimiter{
		requestsPerSecond: requestsPerSecond,
		burst:             burst,
		keyFunc:           keyFunc,
		limiters:          make(map[string]*limiterEntry),
		cleanupInterval:   5 * time.Minute,
		cleanupStop:       make(chan struct{}),
		cleanupDone:       make(chan struct{}),
	}

	// Start cleanup goroutine to remove idle limiters
	go limiter.cleanup()

	return limiter
}

// Allow checks if a request should be allowed.
func (l *TokenBucketLimiter) Allow(ctx context.Context, key string) error {
	if key == "" {
		key = "default"
	}

	limiter := l.getLimiter(key)
	if !limiter.Allow() {
		return ErrRateLimitExceeded
	}

	return nil
}

// getLimiter gets or creates a limiter for the given key.
func (l *TokenBucketLimiter) getLimiter(key string) *rate.Limiter {
	l.mu.RLock()
	entry, exists := l.limiters[key]
	l.mu.RUnlock()

	if exists {
		// Update last access time
		l.mu.Lock()
		entry.lastAccess = time.Now()
		l.mu.Unlock()
		return entry.limiter
	}

	// Create new limiter
	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, exists := l.limiters[key]; exists {
		entry.lastAccess = time.Now()
		return entry.limiter
	}

	newLimiter := rate.NewLimiter(rate.Limit(l.requestsPerSecond), l.burst)
	l.limiters[key] = &limiterEntry{
		limiter:    newLimiter,
		lastAccess: time.Now(),
	}
	return newLimiter
}

// cleanup periodically removes idle limiters to prevent memory leaks.
func (l *TokenBucketLimiter) cleanup() {
	defer close(l.cleanupDone)

	ticker := time.NewTicker(l.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			// Remove limiters that haven't been accessed in 2x cleanup interval
			idleThreshold := time.Now().Add(-2 * l.cleanupInterval)
			for key, entry := range l.limiters {
				if entry.lastAccess.Before(idleThreshold) {
					delete(l.limiters, key)
				}
			}
			l.mu.Unlock()

		case <-l.cleanupStop:
			return
		}
	}
}

// Close stops the cleanup goroutine and releases resources.
func (l *TokenBucketLimiter) Close() error {
	close(l.cleanupStop)
	<-l.cleanupDone
	return nil
}

// NoOpLimiter is a rate limiter that allows all requests.
type NoOpLimiter struct{}

// NewNoOpLimiter creates a limiter that allows all requests.
func NewNoOpLimiter() *NoOpLimiter {
	return &NoOpLimiter{}
}

// Allow always returns nil (allows all requests).
func (l *NoOpLimiter) Allow(ctx context.Context, key string) error {
	return nil
}

// Close is a no-op.
func (l *NoOpLimiter) Close() error {
	return nil
}

// Common key extraction functions

// TenantKeyFunc extracts tenant ID from context.
func TenantKeyFunc(ctx context.Context) string {
	if tenantID, ok := ctx.Value("tenant_id").(string); ok {
		return fmt.Sprintf("tenant:%s", tenantID)
	}
	return "tenant:unknown"
}

// IPKeyFunc extracts IP address from context.
func IPKeyFunc(ctx context.Context) string {
	if ip, ok := ctx.Value("remote_ip").(string); ok {
		return fmt.Sprintf("ip:%s", ip)
	}
	return "ip:unknown"
}

// IdentityKeyFunc extracts identity ID from context.
func IdentityKeyFunc(ctx context.Context) string {
	if identityID, ok := ctx.Value("identity_id").(string); ok {
		return fmt.Sprintf("identity:%s", identityID)
	}
	return "identity:unknown"
}

// GetKeyFunc returns the appropriate key function based on the name.
func GetKeyFunc(name string) KeyFunc {
	switch name {
	case "tenant":
		return TenantKeyFunc
	case "ip":
		return IPKeyFunc
	case "identity":
		return IdentityKeyFunc
	default:
		return func(ctx context.Context) string { return "default" }
	}
}
