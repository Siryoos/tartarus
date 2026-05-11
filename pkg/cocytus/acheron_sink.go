package cocytus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// AcheronSink is a durable [Sink] that persists failed records to a Redis
// Stream, acting as a dead-letter queue that survives process restarts.
//
// The stream key defaults to "cocytus:dlq" and can be customised via
// [AcheronSinkConfig.StreamKey]. Each record is stored as a single Redis
// Stream entry with the following fields:
//
//	run_id      – string representation of [Record.RunID]
//	request_id  – string representation of [Record.RequestID]
//	reason      – human-readable failure reason
//	created_at  – RFC3339 timestamp
//	payload     – raw bytes base64-encoded via JSON marshalling
//
// NOTE: pkg/acheron already imports pkg/cocytus, so this sink intentionally
// talks directly to Redis to avoid a circular import.
type AcheronSink struct {
	client    *redis.Client
	streamKey string
}

// AcheronSinkConfig holds the options for [NewAcheronSink].
type AcheronSinkConfig struct {
	// Addr is the Redis address in "host:port" form (required).
	Addr string
	// DB is the Redis logical database index (0 is the default).
	DB int
	// StreamKey is the Redis stream key to write records to.
	// Defaults to "cocytus:dlq".
	StreamKey string
	// DialTimeout is the timeout used when establishing the initial connection.
	// Defaults to 5 seconds.
	DialTimeout time.Duration
}

// NewAcheronSink creates an AcheronSink and validates the Redis connection.
func NewAcheronSink(cfg AcheronSinkConfig) (*AcheronSink, error) {
	if cfg.StreamKey == "" {
		cfg.StreamKey = "cocytus:dlq"
	}
	dialTimeout := cfg.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	client := redis.NewClient(&redis.Options{
		Addr:        cfg.Addr,
		DB:          cfg.DB,
		DialTimeout: dialTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("cocytus: AcheronSink failed to connect to Redis at %s: %w", cfg.Addr, err)
	}

	return &AcheronSink{
		client:    client,
		streamKey: cfg.StreamKey,
	}, nil
}

// Write persists the record to the Redis DLQ stream. The operation is
// context-aware and will abort if the context is cancelled.
func (s *AcheronSink) Write(ctx context.Context, rec *Record) error {
	payload, err := json.Marshal(rec.Payload)
	if err != nil {
		return fmt.Errorf("cocytus: AcheronSink failed to marshal payload: %w", err)
	}

	args := &redis.XAddArgs{
		Stream: s.streamKey,
		Values: map[string]interface{}{
			"run_id":     string(rec.RunID),
			"request_id": string(rec.RequestID),
			"reason":     rec.Reason,
			"created_at": rec.CreatedAt.UTC().Format(time.RFC3339),
			"payload":    string(payload),
		},
	}

	if err := s.client.XAdd(ctx, args).Err(); err != nil {
		return fmt.Errorf("cocytus: AcheronSink failed to XADD to stream %q: %w", s.streamKey, err)
	}
	return nil
}

// Close releases the underlying Redis connection.
func (s *AcheronSink) Close() error {
	return s.client.Close()
}
