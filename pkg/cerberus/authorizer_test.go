package cerberus

import (
	"context"
	"testing"
)

func TestAllowAllAuthorizer(t *testing.T) {
	ctx := context.Background()
	authz := NewAllowAllAuthorizer()

	identity := &Identity{
		ID:   "test-user",
		Type: IdentityTypeUser,
	}

	resource := Resource{
		Type: ResourceTypeSandbox,
		ID:   "sandbox-123",
	}

	// Should allow any action
	err := authz.Authorize(ctx, identity, ActionCreate, resource)
	if err != nil {
		t.Errorf("AllowAllAuthorizer should allow all requests, got error: %v", err)
	}

	err = authz.Authorize(ctx, identity, ActionDelete, resource)
	if err != nil {
		t.Errorf("AllowAllAuthorizer should allow all requests, got error: %v", err)
	}
}

func TestDenyAllAuthorizer(t *testing.T) {
	ctx := context.Background()
	authz := NewDenyAllAuthorizer()

	identity := &Identity{
		ID:   "test-user",
		Type: IdentityTypeUser,
	}

	resource := Resource{
		Type: ResourceTypeSandbox,
		ID:   "sandbox-123",
	}

	// Should deny any action
	err := authz.Authorize(ctx, identity, ActionCreate, resource)
	if err == nil {
		t.Error("DenyAllAuthorizer should deny all requests, got nil error")
	}
}

func TestRBACAuthorizer(t *testing.T) {
	ctx := context.Background()

	// Define policies
	policies := map[string]*RBACPolicy{
		"admin": {
			Role: "admin",
			Permissions: []Permission{
				{
					AllowAll: true,
				},
			},
		},
		"user": {
			Role: "user",
			Permissions: []Permission{
				{
					Actions:   []Action{ActionCreate, ActionRead, ActionDelete},
					Resources: []ResourceType{ResourceTypeSandbox},
				},
				{
					Actions:   []Action{ActionRead},
					Resources: []ResourceType{ResourceTypeTemplate},
				},
			},
		},
		"readonly": {
			Role: "readonly",
			Permissions: []Permission{
				{
					Actions:   []Action{ActionRead},
					Resources: []ResourceType{ResourceTypeSandbox, ResourceTypeTemplate},
				},
			},
		},
	}

	authz := NewRBACAuthorizer(policies)

	tests := []struct {
		name     string
		identity *Identity
		action   Action
		resource Resource
		wantErr  bool
	}{
		{
			name: "admin can do anything",
			identity: &Identity{
				ID:    "admin-user",
				Roles: []string{"admin"},
			},
			action:   ActionDelete,
			resource: Resource{Type: ResourceTypeSandbox, ID: "sandbox-123"},
			wantErr:  false,
		},
		{
			name: "user can create sandbox",
			identity: &Identity{
				ID:    "regular-user",
				Roles: []string{"user"},
			},
			action:   ActionCreate,
			resource: Resource{Type: ResourceTypeSandbox, ID: "sandbox-123"},
			wantErr:  false,
		},
		{
			name: "user can read template",
			identity: &Identity{
				ID:    "regular-user",
				Roles: []string{"user"},
			},
			action:   ActionRead,
			resource: Resource{Type: ResourceTypeTemplate, ID: "template-1"},
			wantErr:  false,
		},
		{
			name: "user cannot update template",
			identity: &Identity{
				ID:    "regular-user",
				Roles: []string{"user"},
			},
			action:   ActionUpdate,
			resource: Resource{Type: ResourceTypeTemplate, ID: "template-1"},
			wantErr:  true,
		},
		{
			name: "readonly can only read",
			identity: &Identity{
				ID:    "readonly-user",
				Roles: []string{"readonly"},
			},
			action:   ActionRead,
			resource: Resource{Type: ResourceTypeSandbox, ID: "sandbox-123"},
			wantErr:  false,
		},
		{
			name: "readonly cannot create",
			identity: &Identity{
				ID:    "readonly-user",
				Roles: []string{"readonly"},
			},
			action:   ActionCreate,
			resource: Resource{Type: ResourceTypeSandbox, ID: "sandbox-123"},
			wantErr:  true,
		},
		{
			name: "user with no matching role is denied",
			identity: &Identity{
				ID:    "no-role-user",
				Roles: []string{"unknown-role"},
			},
			action:   ActionRead,
			resource: Resource{Type: ResourceTypeSandbox, ID: "sandbox-123"},
			wantErr:  true,
		},
		{
			name: "user with multiple roles uses first matching",
			identity: &Identity{
				ID:    "multi-role-user",
				Roles: []string{"readonly", "user"},
			},
			action:   ActionCreate,
			resource: Resource{Type: ResourceTypeSandbox, ID: "sandbox-123"},
			wantErr:  false, // user role allows create
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authz.Authorize(ctx, tt.identity, tt.action, tt.resource)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
