package typhon

import (
	"context"
	"testing"
)

func TestInMemoryQuarantineManager(t *testing.T) {
	m := NewInMemoryQuarantineManager()
	ctx := context.Background()

	// Test Quarantine
	req := &QuarantineRequest{
		SandboxID:   "sb-123",
		Reason:      ReasonSuspiciousBehavior,
		RequestedBy: "admin",
	}
	record, err := m.Quarantine(ctx, req)
	if err != nil {
		t.Fatalf("Quarantine failed: %v", err)
	}
	if record.Status != StatusActive {
		t.Errorf("Expected status Active, got %v", record.Status)
	}

	// Test List
	list, err := m.ListQuarantined(ctx, &QuarantineFilter{SandboxID: "sb-123"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 record, got %d", len(list))
	}

	// Test Release
	err = m.Release(ctx, "sb-123", &ReleaseApproval{ApprovedBy: "admin"})
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Verify Released Status
	list, _ = m.ListQuarantined(ctx, &QuarantineFilter{SandboxID: "sb-123"})
	if list[0].Status != StatusReleased {
		t.Errorf("Expected status Released, got %v", list[0].Status)
	}
}
