package persephone

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

// HistoryStore persists usage records for learning and forecasting
type HistoryStore interface {
	// Save stores a batch of usage records
	Save(ctx context.Context, records []*UsageRecord) error

	// Load retrieves all records within a time window
	Load(ctx context.Context, start, end time.Time) ([]*UsageRecord, error)

	// QueryRecent retrieves the N most recent records
	QueryRecent(ctx context.Context, count int) ([]*UsageRecord, error)

	// Prune removes records older than the retention period
	Prune(ctx context.Context, retentionDays int) error

	// Close closes the storage backend
	Close() error
}

// RedisHistoryStore stores usage history in Redis using sorted sets
type RedisHistoryStore struct {
	client *redis.Client
	key    string // Redis key for the sorted set
}

func NewRedisHistoryStore(addr string, db int, password string) (*RedisHistoryStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		DB:       db,
		Password: password,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisHistoryStore{
		client: client,
		key:    "persephone:history",
	}, nil
}

func (s *RedisHistoryStore) Save(ctx context.Context, records []*UsageRecord) error {
	if len(records) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()
	for _, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("failed to marshal record: %w", err)
		}

		// Use timestamp as score for time-series indexing
		score := float64(record.Timestamp.Unix())
		pipe.ZAdd(ctx, s.key, redis.Z{
			Score:  score,
			Member: data,
		})
	}

	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisHistoryStore) Load(ctx context.Context, start, end time.Time) ([]*UsageRecord, error) {
	min := float64(start.Unix())
	max := float64(end.Unix())

	results, err := s.client.ZRangeByScore(ctx, s.key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", min),
		Max: fmt.Sprintf("%f", max),
	}).Result()
	if err != nil {
		return nil, err
	}

	records := make([]*UsageRecord, 0, len(results))
	for _, data := range results {
		var record UsageRecord
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			continue // Skip malformed records
		}
		records = append(records, &record)
	}

	return records, nil
}

func (s *RedisHistoryStore) QueryRecent(ctx context.Context, count int) ([]*UsageRecord, error) {
	// Get the N most recent entries (highest scores)
	results, err := s.client.ZRevRange(ctx, s.key, 0, int64(count-1)).Result()
	if err != nil {
		return nil, err
	}

	records := make([]*UsageRecord, 0, len(results))
	for _, data := range results {
		var record UsageRecord
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			continue
		}
		records = append(records, &record)
	}

	// Reverse to get chronological order
	for i := 0; i < len(records)/2; i++ {
		records[i], records[len(records)-1-i] = records[len(records)-1-i], records[i]
	}

	return records, nil
}

func (s *RedisHistoryStore) Prune(ctx context.Context, retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	maxScore := float64(cutoff.Unix())

	_, err := s.client.ZRemRangeByScore(ctx, s.key, "-inf", fmt.Sprintf("%f", maxScore)).Result()
	return err
}

func (s *RedisHistoryStore) Close() error {
	return s.client.Close()
}

// LocalHistoryStore stores usage history in JSON files
type LocalHistoryStore struct {
	dataDir string
	file    string
}

func NewLocalHistoryStore(dataDir string) (*LocalHistoryStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	return &LocalHistoryStore{
		dataDir: dataDir,
		file:    filepath.Join(dataDir, "persephone_history.json"),
	}, nil
}

func (s *LocalHistoryStore) Save(ctx context.Context, records []*UsageRecord) error {
	// Load existing records
	existing, err := s.loadFromFile()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Append new records
	existing = append(existing, records...)

	// Sort by timestamp
	sort.Slice(existing, func(i, j int) bool {
		return existing[i].Timestamp.Before(existing[j].Timestamp)
	})

	// Write back to file
	return s.saveToFile(existing)
}

func (s *LocalHistoryStore) Load(ctx context.Context, start, end time.Time) ([]*UsageRecord, error) {
	all, err := s.loadFromFile()
	if err != nil {
		if os.IsNotExist(err) {
			return []*UsageRecord{}, nil
		}
		return nil, err
	}

	// Filter by time range
	filtered := make([]*UsageRecord, 0)
	for _, record := range all {
		if (record.Timestamp.Equal(start) || record.Timestamp.After(start)) &&
			(record.Timestamp.Equal(end) || record.Timestamp.Before(end)) {
			filtered = append(filtered, record)
		}
	}

	return filtered, nil
}

func (s *LocalHistoryStore) QueryRecent(ctx context.Context, count int) ([]*UsageRecord, error) {
	all, err := s.loadFromFile()
	if err != nil {
		if os.IsNotExist(err) {
			return []*UsageRecord{}, nil
		}
		return nil, err
	}

	// Get last N records
	if len(all) <= count {
		return all, nil
	}

	return all[len(all)-count:], nil
}

func (s *LocalHistoryStore) Prune(ctx context.Context, retentionDays int) error {
	all, err := s.loadFromFile()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	// Filter out old records
	kept := make([]*UsageRecord, 0)
	for _, record := range all {
		if record.Timestamp.After(cutoff) {
			kept = append(kept, record)
		}
	}

	return s.saveToFile(kept)
}

func (s *LocalHistoryStore) Close() error {
	return nil // Nothing to close for file storage
}

func (s *LocalHistoryStore) loadFromFile() ([]*UsageRecord, error) {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return nil, err
	}

	var records []*UsageRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("failed to unmarshal history: %w", err)
	}

	return records, nil
}

func (s *LocalHistoryStore) saveToFile(records []*UsageRecord) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	return os.WriteFile(s.file, data, 0644)
}
