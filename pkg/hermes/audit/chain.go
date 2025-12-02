package audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// ChainManager handles the cryptographic chaining of audit events.
type ChainManager struct {
	secretKey []byte
}

// NewChainManager creates a new ChainManager with the given secret key.
func NewChainManager(secretKey []byte) *ChainManager {
	return &ChainManager{
		secretKey: secretKey,
	}
}

// ComputeHash computes the HMAC-SHA256 hash of the event.
// It assumes PreviousHash is already set on the event.
func (c *ChainManager) ComputeHash(event *Event) (string, error) {
	// Create a copy to avoid modifying the original event during marshaling if needed,
	// but here we just want to hash the content.
	// We exclude the Hash field itself from the payload to be hashed.

	// A simple way is to create a struct that matches Event but without Hash
	// or just manually build the payload. For robustness, let's marshal a map
	// or a specific struct.

	// To ensure deterministic hashing, we need a canonical representation.
	// JSON marshaling in Go sorts map keys, which helps.

	payload := struct {
		ID           string                 `json:"id"`
		Timestamp    string                 `json:"timestamp"` // Use string for fixed format
		Action       Action                 `json:"action"`
		Result       Result                 `json:"result"`
		Resource     Resource               `json:"resource"`
		Identity     *Identity              `json:"identity,omitempty"`
		SourceIP     string                 `json:"source_ip,omitempty"`
		UserAgent    string                 `json:"user_agent,omitempty"`
		RequestID    string                 `json:"request_id,omitempty"`
		Latency      int64                  `json:"latency,omitempty"` // Nanoseconds
		ErrorMessage string                 `json:"error_message,omitempty"`
		Metadata     map[string]interface{} `json:"metadata,omitempty"`
		PreviousHash string                 `json:"previous_hash,omitempty"`
	}{
		ID:           event.ID,
		Timestamp:    event.Timestamp.UTC().Format(time.RFC3339Nano),
		Action:       event.Action,
		Result:       event.Result,
		Resource:     event.Resource,
		Identity:     event.Identity,
		SourceIP:     event.SourceIP,
		UserAgent:    event.UserAgent,
		RequestID:    event.RequestID,
		Latency:      event.Latency.Nanoseconds(),
		ErrorMessage: event.ErrorMessage,
		Metadata:     event.Metadata,
		PreviousHash: event.PreviousHash,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event for hashing: %w", err)
	}

	h := hmac.New(sha256.New, c.secretKey)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyChain verifies the integrity of a slice of events.
func (c *ChainManager) VerifyChain(events []Event) error {
	if len(events) == 0 {
		return nil
	}

	for i, event := range events {
		// Verify current event hash
		expectedHash, err := c.ComputeHash(&event)
		if err != nil {
			return fmt.Errorf("failed to compute hash for event %s: %w", event.ID, err)
		}
		if event.Hash != expectedHash {
			return fmt.Errorf("hash mismatch for event %s: expected %s, got %s", event.ID, expectedHash, event.Hash)
		}

		// Verify chain link
		if i > 0 {
			if event.PreviousHash != events[i-1].Hash {
				return fmt.Errorf("chain broken at event %s: previous hash %s does not match hash of event %s (%s)",
					event.ID, event.PreviousHash, events[i-1].ID, events[i-1].Hash)
			}
		}
	}

	return nil
}
