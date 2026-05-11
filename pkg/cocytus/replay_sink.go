package cocytus

import (
	"context"
	"fmt"
	"time"
)

// ReplaySink is a [Sink] decorator that adds bounded retry and a dead-letter
// fallback to any inner [Sink]. It implements the replay capability required
// for production-grade error stream handling.
//
// On every [Write] call the inner sink is attempted up to [ReplayConfig.MaxAttempts]
// times. Between each attempt the caller blocks for [ReplayConfig.Backoff].
// If all attempts are exhausted the record is forwarded to [ReplayConfig.Fallback]
// (if non-nil) before returning the last error. This lets operators chain a
// durable sink as the fallback:
//
//	dlq, _ := cocytus.NewAcheronSink(cfg)
//	replayer := cocytus.NewReplaySink(cocytus.ReplayConfig{
//	    Inner:       myFlakySink,
//	    MaxAttempts: 3,
//	    Backoff:     500 * time.Millisecond,
//	    Fallback:    dlq,
//	})
type ReplaySink struct {
	inner       Sink
	maxAttempts int
	backoff     time.Duration
	fallback    Sink
}

// ReplayConfig controls the retry and fallback behaviour of [ReplaySink].
type ReplayConfig struct {
	// Inner is the primary Sink to attempt (required).
	Inner Sink
	// MaxAttempts is the total number of delivery attempts including the first
	// try. Must be ≥ 1. Defaults to 3.
	MaxAttempts int
	// Backoff is the fixed wait duration between consecutive attempts.
	// Defaults to 500ms.
	Backoff time.Duration
	// Fallback is the Sink to write to when all attempts are exhausted.
	// If nil, the last error is returned without further action.
	Fallback Sink
}

// NewReplaySink creates a ReplaySink with the provided configuration.
func NewReplaySink(cfg ReplayConfig) (*ReplaySink, error) {
	if cfg.Inner == nil {
		return nil, fmt.Errorf("cocytus: ReplaySink requires a non-nil Inner sink")
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = 500 * time.Millisecond
	}
	return &ReplaySink{
		inner:       cfg.Inner,
		maxAttempts: cfg.MaxAttempts,
		backoff:     cfg.Backoff,
		fallback:    cfg.Fallback,
	}, nil
}

// Write attempts to deliver rec to the inner sink, retrying on transient
// failures up to MaxAttempts times. If all attempts fail and a Fallback sink
// is configured, the record is forwarded to it as a last resort.
func (r *ReplaySink) Write(ctx context.Context, rec *Record) error {
	var lastErr error
	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := r.inner.Write(ctx, rec); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if attempt < r.maxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.backoff):
			}
		}
	}

	// All attempts exhausted — forward to fallback if available.
	if r.fallback != nil {
		if fbErr := r.fallback.Write(ctx, rec); fbErr != nil {
			return fmt.Errorf("cocytus: ReplaySink exhausted %d attempts (last: %w); fallback also failed: %v",
				r.maxAttempts, lastErr, fbErr)
		}
		return fmt.Errorf("cocytus: ReplaySink exhausted %d attempts (last: %w); record forwarded to fallback",
			r.maxAttempts, lastErr)
	}

	return fmt.Errorf("cocytus: ReplaySink exhausted %d attempts: %w", r.maxAttempts, lastErr)
}
