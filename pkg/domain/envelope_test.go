package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestEnvelopeMarshalUnmarshal(t *testing.T) {
	payload := domain.ResourceCapacity{
		CPU: 1000,
		Mem: 2048,
		GPU: 1,
	}

	data, err := domain.MarshalEnvelope("ResourceCapacity", 1, payload)
	if err != nil {
		t.Fatalf("MarshalEnvelope failed: %v", err)
	}

	gotPayload, version, err := domain.UnmarshalEnvelope[domain.ResourceCapacity](data, "ResourceCapacity")
	if err != nil {
		t.Fatalf("UnmarshalEnvelope failed: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
	if gotPayload != payload {
		t.Errorf("expected payload %v, got %v", payload, gotPayload)
	}
}

func TestEnvelopeUnmarshalLegacy(t *testing.T) {
	payload := domain.ResourceCapacity{
		CPU: 500,
		Mem: 1024,
	}
	
	// Create legacy unversioned JSON
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	gotPayload, version, err := domain.UnmarshalEnvelope[domain.ResourceCapacity](data, "ResourceCapacity")
	if err != nil {
		t.Fatalf("UnmarshalEnvelope failed on legacy data: %v", err)
	}
	
	if version != 0 {
		t.Errorf("expected version 0 for legacy data, got %d", version)
	}
	if gotPayload != payload {
		t.Errorf("expected payload %v, got %v", payload, gotPayload)
	}
}

func TestEnvelopeKindMismatch(t *testing.T) {
	payload := domain.ResourceCapacity{CPU: 100}
	data, _ := domain.MarshalEnvelope("ResourceCapacity", 1, payload)

	_, _, err := domain.UnmarshalEnvelope[domain.ResourceCapacity](data, "WrongKind")
	if err == nil {
		t.Fatalf("expected error for kind mismatch, got nil")
	}
}
