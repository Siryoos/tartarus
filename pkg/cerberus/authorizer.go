package cerberus

import "context"

// Authorizer checks if an identity has permission to perform an action on a resource.
type Authorizer interface {
	Authorize(ctx context.Context, identity *Identity, action Action, resource Resource) error
}

// AllowAllAuthorizer permits all requests.
// This is useful for development or when authorization is not yet required.
type AllowAllAuthorizer struct{}

// NewAllowAllAuthorizer creates a permissive authorizer.
func NewAllowAllAuthorizer() *AllowAllAuthorizer {
	return &AllowAllAuthorizer{}
}

// Authorize always returns nil (allows all requests).
func (a *AllowAllAuthorizer) Authorize(ctx context.Context, identity *Identity, action Action, resource Resource) error {
	return nil
}

// RBACAuthorizer implements role-based access control.
type RBACAuthorizer struct {
	policies map[string]*RBACPolicy // Map of role to policy
}

// RBACPolicy defines permissions for a role.
type RBACPolicy struct {
	Role        string       `yaml:"role" json:"role"`
	Permissions []Permission `yaml:"permissions" json:"permissions"`
}

// Permission defines what actions are allowed on what resources.
type Permission struct {
	Actions   []Action       `yaml:"actions" json:"actions"`
	Resources []ResourceType `yaml:"resources" json:"resources"`
	AllowAll  bool           `yaml:"allowAll" json:"allow_all"`
}

// NewRBACAuthorizer creates a role-based authorizer.
func NewRBACAuthorizer(policies map[string]*RBACPolicy) *RBACAuthorizer {
	return &RBACAuthorizer{
		policies: policies,
	}
}

// Authorize checks if the identity's roles grant permission for the action.
func (r *RBACAuthorizer) Authorize(ctx context.Context, identity *Identity, action Action, resource Resource) error {
	// Check each role the identity has
	for _, role := range identity.Roles {
		policy, exists := r.policies[role]
		if !exists {
			continue
		}

		// Check if any permission in this policy allows the action
		for _, perm := range policy.Permissions {
			if perm.AllowAll {
				return nil // Full access
			}

			// Check if action is allowed
			actionAllowed := false
			for _, allowedAction := range perm.Actions {
				if allowedAction == action || allowedAction == ActionAll {
					actionAllowed = true
					break
				}
			}

			if !actionAllowed {
				continue
			}

			// Check if resource type is allowed
			resourceAllowed := false
			for _, allowedResource := range perm.Resources {
				if allowedResource == resource.Type || allowedResource == ResourceTypeAll {
					resourceAllowed = true
					break
				}
			}

			if !resourceAllowed {
				continue
			}

			// Check tenant scoping
			// If the resource has a tenant ID, we must ensure the identity is allowed to access it.
			// For now, we assume simple model:
			// 1. If identity.TenantID matches resource.TenantID, allow.
			// 2. If identity is "admin" (or has global access), allow.
			// 3. If resource is public (no tenant), allow.

			// If resource has no tenant, it's global/public
			if resource.TenantID == "" {
				return nil
			}

			// If identity has no tenant, they might be a super-admin or unauthenticated (if we allow that)
			// But here we are in Authorize, so we assume Authenticate passed.
			// If identity.TenantID is empty, maybe they are system admin?
			// Let's assume empty identity.TenantID means "no tenant context", so deny unless role is specific.
			// But wait, we don't have "IsAdmin" flag on identity.
			// We rely on roles.
			// If the policy allows the action/resource, does it imply tenant access?
			// Usually RBAC is orthogonal to tenancy, or tenancy is a filter.
			// Let's enforce: Identity must belong to the same tenant as the resource,
			// UNLESS the role is a "system" role that spans tenants.
			// How do we know if a role is system-wide?
			// For now, let's hardcode "admin" role as system-wide.
			if role == "admin" {
				return nil
			}

			if identity.TenantID == resource.TenantID {
				return nil
			}

			// If we get here, the permission matched action/resource, but tenant check failed.
			// We continue to next permission/role in case another one grants access.
		}
	}

	// No role granted permission
	return NewAuthorizationError("insufficient permissions", identity, action, resource)
}

// DenyAllAuthorizer denies all requests.
// This is useful for maintenance mode or emergency lockdown.
type DenyAllAuthorizer struct{}

// NewDenyAllAuthorizer creates a restrictive authorizer.
func NewDenyAllAuthorizer() *DenyAllAuthorizer {
	return &DenyAllAuthorizer{}
}

// Authorize always returns an authorization error.
func (d *DenyAllAuthorizer) Authorize(ctx context.Context, identity *Identity, action Action, resource Resource) error {
	return NewAuthorizationError("all access denied (maintenance mode)", identity, action, resource)
}
