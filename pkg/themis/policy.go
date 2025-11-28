package themis

import (
	"context"

	"github.com/tartarus-sandbox/tartarus/pkg/domain"
)

// Repository manages sandbox policies.

type Repository interface {
	GetPolicy(ctx context.Context, tplID domain.TemplateID) (*domain.SandboxPolicy, error)
	UpsertPolicy(ctx context.Context, p *domain.SandboxPolicy) error
	ListPolicies(ctx context.Context) ([]*domain.SandboxPolicy, error)
}

// Validator checks a request against policy.

type Validator interface {
	ValidateRequest(ctx context.Context, req *domain.SandboxRequest, p *domain.SandboxPolicy) error
}
