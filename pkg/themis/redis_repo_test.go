package themis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

func TestRedisRepo_UpsertGet(t *testing.T) {
	s := miniredis.RunT(t)
	repo, err := NewRedisRepo(s.Addr(), 0, "")
	if err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}

	ctx := context.Background()
	tplID := domain.TemplateID("tpl-1")
	policy := &domain.SandboxPolicy{
		ID:         "pol-1",
		TemplateID: tplID,
		Resources: domain.ResourceSpec{
			CPU: 500,
			Mem: 64,
		},
		Version: 0, // New policy
	}

	// 1. Create new policy
	if err := repo.UpsertPolicy(ctx, policy); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if policy.Version != 1 {
		t.Errorf("Expected version 1, got %d", policy.Version)
	}

	// 2. Get policy
	got, err := repo.GetPolicy(ctx, tplID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != policy.ID {
		t.Errorf("Expected ID %s, got %s", policy.ID, got.ID)
	}
	if got.Version != 1 {
		t.Errorf("Expected fetched version 1, got %d", got.Version)
	}

	// 3. Update policy (success)
	got.Resources.CPU = 1000
	if err := repo.UpsertPolicy(ctx, got); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Expected version 2, got %d", got.Version)
	}

	// 4. Optimistic lock failure
	// Try to update with old version (1)
	oldPolicy := &domain.SandboxPolicy{
		ID:         "pol-1",
		TemplateID: tplID,
		Version:    1,
	}
	if err := repo.UpsertPolicy(ctx, oldPolicy); err == nil {
		t.Error("Expected optimistic lock failure, got nil")
	}
}

func TestRedisRepo_List(t *testing.T) {
	s := miniredis.RunT(t)
	repo, err := NewRedisRepo(s.Addr(), 0, "")
	if err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}

	ctx := context.Background()

	// Create 2 policies
	p1 := &domain.SandboxPolicy{TemplateID: "tpl-1", ID: "pol-1"}
	p2 := &domain.SandboxPolicy{TemplateID: "tpl-2", ID: "pol-2"}

	if err := repo.UpsertPolicy(ctx, p1); err != nil {
		t.Fatalf("Upsert p1 failed: %v", err)
	}
	if err := repo.UpsertPolicy(ctx, p2); err != nil {
		t.Fatalf("Upsert p2 failed: %v", err)
	}

	list, err := repo.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("Expected 2 policies, got %d", len(list))
	}
}
