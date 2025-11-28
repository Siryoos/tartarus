package themis

import (
	"context"
	"sync"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// MemoryRepo is an in-memory implementation of the Repository interface.
type MemoryRepo struct {
	mu      sync.RWMutex
	byTplID map[domain.TemplateID]*domain.SandboxPolicy
}

// NewMemoryRepo creates a new in-memory policy repository.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		byTplID: make(map[domain.TemplateID]*domain.SandboxPolicy),
	}
}

// GetPolicy retrieves a policy for the given template ID.
// If no policy exists, it returns a default lockdown policy.
func (r *MemoryRepo) GetPolicy(ctx context.Context, tplID domain.TemplateID) (*domain.SandboxPolicy, error) {
	r.mu.RLock()
	policy, exists := r.byTplID[tplID]
	r.mu.RUnlock()

	if exists {
		return policy, nil
	}

	// Return default lockdown policy
	return &domain.SandboxPolicy{
		ID:         domain.PolicyID("lockdown-default"),
		TemplateID: tplID,
		Resources: domain.ResourceSpec{
			CPU: 1000, // 1 CPU core (1000 milliCPU)
			Mem: 128,  // 128 MB
		},
		NetworkPolicy: domain.NetworkPolicyRef{
			ID:   "lockdown-no-net",
			Name: "No Internet",
		},
		Tags: map[string]string{
			"type": "default-lockdown",
		},
	}, nil
}

// UpsertPolicy inserts or updates a policy in the repository.
func (r *MemoryRepo) UpsertPolicy(ctx context.Context, p *domain.SandboxPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byTplID[p.TemplateID] = p
	return nil
}

// ListPolicies returns all stored policies.
func (r *MemoryRepo) ListPolicies(ctx context.Context) ([]*domain.SandboxPolicy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	policies := make([]*domain.SandboxPolicy, 0, len(r.byTplID))
	for _, p := range r.byTplID {
		policies = append(policies, p)
	}

	return policies, nil
}
