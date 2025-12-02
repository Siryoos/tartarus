package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// Store defines the interface for persisting audit events.
type Store interface {
	Write(ctx context.Context, event *Event) error
	// Read is a placeholder for future retrieval logic
	// Read(ctx context.Context, filter Filter) ([]Event, error)
}

// LogStore writes audit events to a simple log file or writer.
// It is safe for concurrent use.
type LogStore struct {
	mu     sync.Mutex
	writer io.Writer
}

// NewLogStore creates a new LogStore writing to the provided writer.
func NewLogStore(w io.Writer) *LogStore {
	return &LogStore{
		writer: w,
	}
}

// NewFileStore creates a new LogStore writing to a file.
func NewFileStore(path string) (*LogStore, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return NewLogStore(f), nil
}

// Write writes the event to the underlying writer as a JSON line.
func (s *LogStore) Write(ctx context.Context, event *Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.writer.Write(append(data, '\n'))
	return err
}

// TamperEvidentStore wraps a Store and adds HMAC chaining.
type TamperEvidentStore struct {
	store        Store
	chainManager *ChainManager
	lastHash     string
	mu           sync.Mutex
}

// NewTamperEvidentStore creates a new TamperEvidentStore.
func NewTamperEvidentStore(store Store, chainManager *ChainManager) *TamperEvidentStore {
	return &TamperEvidentStore{
		store:        store,
		chainManager: chainManager,
	}
}

// Write computes the hash for the event (chaining it to the previous one) and writes it to the underlying store.
func (s *TamperEvidentStore) Write(ctx context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	event.PreviousHash = s.lastHash
	hash, err := s.chainManager.ComputeHash(event)
	if err != nil {
		return err
	}
	event.Hash = hash
	s.lastHash = hash

	return s.store.Write(ctx, event)
}
