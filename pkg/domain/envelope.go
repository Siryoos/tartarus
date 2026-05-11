package domain

import (
	"encoding/json"
	"fmt"
)

const (
	SchemaVersionNodeStatus = 1
	SchemaVersionSandboxRun = 1
)

// Envelope provides a versioned wrapper for JSON serialised structs.
// This allows schema evolution for structs stored in Redis or sent over the wire.
type Envelope[T any] struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Payload       T      `json:"payload"`
}

// MarshalEnvelope wraps a value in an Envelope and marshals it to JSON.
func MarshalEnvelope[T any](kind string, version int, payload T) ([]byte, error) {
	env := Envelope[T]{
		SchemaVersion: version,
		Kind:          kind,
		Payload:       payload,
	}
	return json.Marshal(env)
}

// UnmarshalEnvelope unmarshals JSON data into an Envelope and returns the payload and version.
// If the data does not appear to be an envelope (e.g. legacy data without schema_version),
// it falls back to unmarshaling directly into the payload type and returns version 0.
func UnmarshalEnvelope[T any](data []byte, expectedKind string) (T, int, error) {
	var env Envelope[T]
	if err := json.Unmarshal(data, &env); err != nil {
		var zero T
		return zero, 0, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}

	// If SchemaVersion is 0, it means the field wasn't present in the JSON.
	// This is our heuristic for detecting legacy unversioned data.
	if env.SchemaVersion == 0 {
		var legacyPayload T
		if err := json.Unmarshal(data, &legacyPayload); err != nil {
			return legacyPayload, 0, fmt.Errorf("failed to unmarshal legacy payload: %w", err)
		}
		return legacyPayload, 0, nil
	}

	if env.Kind != expectedKind {
		var zero T
		return zero, 0, fmt.Errorf("kind mismatch: expected %s, got %s", expectedKind, env.Kind)
	}

	return env.Payload, env.SchemaVersion, nil
}
