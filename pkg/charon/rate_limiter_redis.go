package charon

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisRateLimitScript is a sliding-window rate-limit script.
//
// KEYS[1] — sorted-set key for this (limiter, client-key) pair.
// ARGV[1] — current time in microseconds (Unix).
// ARGV[2] — window size in microseconds.
// ARGV[3] — maximum allowed requests in the window.
// ARGV[4] — TTL for the key in seconds (window * 2 to allow natural expiry).
//
// Returns 1 when the request is allowed, 0 when it is rate-limited.
var redisRateLimitScript = redis.NewScript(`
local key     = KEYS[1]
local now     = tonumber(ARGV[1])
local window  = tonumber(ARGV[2])
local limit   = tonumber(ARGV[3])
local ttl     = tonumber(ARGV[4])

-- Remove entries outside the current window.
redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)

-- Count remaining entries in the window.
local count = redis.call('ZCARD', key)

if count < limit then
    -- Allow: add this request's timestamp as both score and member.
    -- Use now+count as member to handle simultaneous requests.
    redis.call('ZADD', key, now, now .. ':' .. count)
    redis.call('EXPIRE', key, ttl)
    return 1
end

return 0
`)

// RedisRateLimiter implements a globally-shared, sliding-window rate limiter
// backed by Redis. It is safe for use across multiple Charon processes because
// the Lua script executes atomically on the Redis server.
//
// Use NewRedisRateLimiter to construct one; the caller is responsible for
// managing the lifecycle of the redis.UniversalClient.
type RedisRateLimiter struct {
	client            redis.UniversalClient
	requestsPerSecond int
	burst             int
	keyPrefix         string
	keyFunc           KeyFunc

	// Derived from requestsPerSecond; stored to avoid repeated division.
	windowMicros int64
}

// NewRedisRateLimiter creates a sliding-window rate limiter backed by Redis.
//
//   - client            — a connected redis.UniversalClient (standalone, cluster, or sentinel).
//   - requestsPerSecond — sustained requests allowed per second per key.
//   - burst             — peak capacity (the window accommodates up to burst requests).
//   - keyPrefix         — namespace prefix for Redis keys, e.g. "charon:rl".
//   - keyFunc           — function that extracts the rate-limit key from the context.
func NewRedisRateLimiter(
	client redis.UniversalClient,
	requestsPerSecond, burst int,
	keyPrefix string,
	keyFunc KeyFunc,
) *RedisRateLimiter {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 100
	}
	if burst <= 0 {
		burst = requestsPerSecond * 2
	}
	if keyPrefix == "" {
		keyPrefix = "charon:rl"
	}
	if keyFunc == nil {
		keyFunc = func(ctx context.Context) string { return "default" }
	}

	return &RedisRateLimiter{
		client:            client,
		requestsPerSecond: requestsPerSecond,
		burst:             burst,
		keyPrefix:         keyPrefix,
		keyFunc:           keyFunc,
		windowMicros:      int64(time.Second / time.Microsecond),
	}
}

// Allow checks whether the request identified by key is within the rate limit.
// It returns ErrRateLimitExceeded when the global limit across all Charon
// instances has been reached.
func (r *RedisRateLimiter) Allow(ctx context.Context, key string) error {
	if key == "" {
		key = r.keyFunc(ctx)
	}

	redisKey := fmt.Sprintf("%s:%s", r.keyPrefix, key)
	nowMicros := time.Now().UnixMicro()
	ttlSeconds := int64(2) // 2-second key TTL (window is 1 s, 2× for safety)

	result, err := redisRateLimitScript.Run(
		ctx,
		r.client,
		[]string{redisKey},
		nowMicros,
		r.windowMicros,
		r.burst, // use burst as the per-window cap
		ttlSeconds,
	).Int()
	if err != nil {
		// On Redis error, fail open to avoid taking down the service.
		return nil
	}

	if result == 0 {
		return ErrRateLimitExceeded
	}
	return nil
}

// Close is a no-op. The caller owns the redis.UniversalClient lifecycle.
func (r *RedisRateLimiter) Close() error {
	return nil
}

// NewRedisClientFromAddr is a convenience constructor that creates a standalone
// Redis client from a single address string (e.g. "localhost:6379").
func NewRedisClientFromAddr(addr string) redis.UniversalClient {
	return redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{addr},
	})
}
