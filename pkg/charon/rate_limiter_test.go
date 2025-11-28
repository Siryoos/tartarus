package charon

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucketLimiter_AllowWithinLimit(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 10, func(ctx context.Context) string {
		return "test-key"
	})
	defer limiter.Close()

	ctx := context.Background()

	// Should allow requests within limit
	for i := 0; i < 10; i++ {
		if err := limiter.Allow(ctx, "test-key"); err != nil {
			t.Errorf("Request %d should be allowed, got error: %v", i, err)
		}
	}
}

func TestTokenBucketLimiter_ExceedLimit(t *testing.T) {
	limiter := NewTokenBucketLimiter(1, 1, func(ctx context.Context) string {
		return "test-key"
	})
	defer limiter.Close()

	ctx := context.Background()

	// First request should be allowed
	if err := limiter.Allow(ctx, "test-key"); err != nil {
		t.Errorf("First request should be allowed, got error: %v", err)
	}

	// Second request should be rate limited
	if err := limiter.Allow(ctx, "test-key"); err != ErrRateLimitExceeded {
		t.Errorf("Second request should be rate limited, got: %v", err)
	}
}

func TestTokenBucketLimiter_BurstCapacity(t *testing.T) {
	limiter := NewTokenBucketLimiter(1, 5, func(ctx context.Context) string {
		return "test-key"
	})
	defer limiter.Close()

	ctx := context.Background()

	// Should allow burst of 5 requests
	for i := 0; i < 5; i++ {
		if err := limiter.Allow(ctx, "test-key"); err != nil {
			t.Errorf("Burst request %d should be allowed, got error: %v", i, err)
		}
	}

	// 6th request should be rate limited
	if err := limiter.Allow(ctx, "test-key"); err != ErrRateLimitExceeded {
		t.Errorf("Request beyond burst should be rate limited, got: %v", err)
	}
}

func TestTokenBucketLimiter_TokenRefill(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 1, func(ctx context.Context) string {
		return "test-key"
	})
	defer limiter.Close()

	ctx := context.Background()

	// Exhaust tokens
	limiter.Allow(ctx, "test-key")

	// Should be rate limited
	if err := limiter.Allow(ctx, "test-key"); err != ErrRateLimitExceeded {
		t.Error("Should be rate limited after exhausting tokens")
	}

	// Wait for token refill (100ms for 10 req/s = 1 token)
	time.Sleep(150 * time.Millisecond)

	// Should allow request after refill
	if err := limiter.Allow(ctx, "test-key"); err != nil {
		t.Errorf("Should allow request after token refill, got error: %v", err)
	}
}

func TestTokenBucketLimiter_MultipleKeys(t *testing.T) {
	limiter := NewTokenBucketLimiter(1, 1, func(ctx context.Context) string {
		return "test-key"
	})
	defer limiter.Close()

	ctx := context.Background()

	// Exhaust tokens for key1
	limiter.Allow(ctx, "key1")
	if err := limiter.Allow(ctx, "key1"); err != ErrRateLimitExceeded {
		t.Error("key1 should be rate limited")
	}

	// key2 should still be allowed
	if err := limiter.Allow(ctx, "key2"); err != nil {
		t.Errorf("key2 should be allowed, got error: %v", err)
	}
}

func TestTokenBucketLimiter_EmptyKey(t *testing.T) {
	limiter := NewTokenBucketLimiter(1, 1, func(ctx context.Context) string {
		return "test-key"
	})
	defer limiter.Close()

	ctx := context.Background()

	// Empty key should use "default"
	if err := limiter.Allow(ctx, ""); err != nil {
		t.Errorf("Empty key should be allowed, got error: %v", err)
	}
}

func TestNoOpLimiter(t *testing.T) {
	limiter := NewNoOpLimiter()
	defer limiter.Close()

	ctx := context.Background()

	// Should always allow requests
	for i := 0; i < 100; i++ {
		if err := limiter.Allow(ctx, "test-key"); err != nil {
			t.Errorf("NoOp limiter should always allow requests, got error: %v", err)
		}
	}
}

func TestKeyFunctions(t *testing.T) {
	tests := []struct {
		name     string
		keyFunc  KeyFunc
		ctxValue map[string]interface{}
		expected string
	}{
		{
			name:     "TenantKeyFunc with tenant",
			keyFunc:  TenantKeyFunc,
			ctxValue: map[string]interface{}{"tenant_id": "tenant-123"},
			expected: "tenant:tenant-123",
		},
		{
			name:     "TenantKeyFunc without tenant",
			keyFunc:  TenantKeyFunc,
			ctxValue: map[string]interface{}{},
			expected: "tenant:unknown",
		},
		{
			name:     "IPKeyFunc with IP",
			keyFunc:  IPKeyFunc,
			ctxValue: map[string]interface{}{"remote_ip": "192.168.1.1"},
			expected: "ip:192.168.1.1",
		},
		{
			name:     "IdentityKeyFunc with identity",
			keyFunc:  IdentityKeyFunc,
			ctxValue: map[string]interface{}{"identity_id": "user-456"},
			expected: "identity:user-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			for key, value := range tt.ctxValue {
				ctx = context.WithValue(ctx, key, value)
			}

			result := tt.keyFunc(ctx)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetKeyFunc(t *testing.T) {
	tests := []struct {
		name     string
		funcName string
		expected string
	}{
		{"tenant", "tenant", "tenant:unknown"},
		{"ip", "ip", "ip:unknown"},
		{"identity", "identity", "identity:unknown"},
		{"default", "unknown", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyFunc := GetKeyFunc(tt.funcName)
			result := keyFunc(context.Background())
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
