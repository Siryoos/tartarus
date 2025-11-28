package cerberus

import (
	"context"
	"time"
)

// Gateway represents the three-headed guardian of Tartarus.
// Each head serves a distinct purpose in access control:
//   - Head 1 (Authenticate): Verifies identity
//   - Head 2 (Authorize): Checks permissions
//   - Head 3 (RecordAccess): Audits all access
type Gateway interface {
	// Authenticate verifies credentials and returns an identity.
	// Returns AuthenticationError if credentials are invalid.
	Authenticate(ctx context.Context, creds Credentials) (*Identity, error)

	// Authorize checks if an identity has permission to perform an action on a resource.
	// Returns AuthorizationError if permission is denied.
	Authorize(ctx context.Context, identity *Identity, action Action, resource Resource) error

	// RecordAccess logs an access attempt for audit and compliance.
	// Should not fail the request even if audit logging fails.
	RecordAccess(ctx context.Context, entry *AuditEntry) error
}

// Credentials represents authentication credentials in various forms.
type Credentials interface {
	Type() CredentialType
}

// CredentialType identifies the authentication method.
type CredentialType string

const (
	CredentialTypeAPIKey   CredentialType = "api_key"
	CredentialTypeOAuth2   CredentialType = "oauth2"
	CredentialTypeMTLS     CredentialType = "mtls"
	CredentialTypeInternal CredentialType = "internal"
)

// APIKeyCredential represents API key-based authentication.
type APIKeyCredential struct {
	KeyID  string
	Secret string
}

func (c *APIKeyCredential) Type() CredentialType { return CredentialTypeAPIKey }

// OAuth2Credential represents OAuth2 token-based authentication.
type OAuth2Credential struct {
	AccessToken string
	TokenType   string
}

func (c *OAuth2Credential) Type() CredentialType { return CredentialTypeOAuth2 }

// Identity represents an authenticated entity.
type Identity struct {
	ID          string            // Unique identifier
	Type        IdentityType      // Type of entity
	TenantID    string            // Multi-tenancy support
	DisplayName string            // Human-readable name
	Roles       []string          // Assigned roles
	Groups      []string          // Group memberships
	Attributes  map[string]string // Additional metadata
	AuthTime    time.Time         // When authenticated
	ExpiresAt   time.Time         // When authentication expires
}

// IdentityType categorizes the authenticated entity.
type IdentityType string

const (
	IdentityTypeUser    IdentityType = "user"
	IdentityTypeService IdentityType = "service"
	IdentityTypeAgent   IdentityType = "agent"
	IdentityTypeSystem  IdentityType = "system"
)

// Action represents an operation being performed.
type Action string

const (
	ActionCreate  Action = "create"
	ActionRead    Action = "read"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionExecute Action = "execute"
	ActionAdmin   Action = "admin"
)

// Resource represents what is being accessed.
type Resource struct {
	Type      ResourceType
	ID        string
	TenantID  string
	Namespace string
}

// ResourceType identifies the kind of resource.
type ResourceType string

const (
	ResourceTypeSandbox  ResourceType = "sandbox"
	ResourceTypeTemplate ResourceType = "template"
	ResourceTypeSnapshot ResourceType = "snapshot"
	ResourceTypePolicy   ResourceType = "policy"
	ResourceTypeNode     ResourceType = "node"
)

// AuditEntry captures access information for compliance and security monitoring.
type AuditEntry struct {
	Timestamp    time.Time
	RequestID    string
	Identity     *Identity
	Action       Action
	Resource     Resource
	Result       AuditResult
	Latency      time.Duration
	SourceIP     string
	UserAgent    string
	ErrorMessage string
}

// AuditResult indicates the outcome of an access attempt.
type AuditResult string

const (
	AuditResultSuccess AuditResult = "success"
	AuditResultDenied  AuditResult = "denied"
	AuditResultError   AuditResult = "error"
)

// DefaultGateway implements the Gateway interface by composing
// an Authenticator, Authorizer, and Auditor.
type DefaultGateway struct {
	authenticator Authenticator
	authorizer    Authorizer
	auditor       Auditor
}

// NewGateway creates a new Gateway with the three heads.
func NewGateway(auth Authenticator, authz Authorizer, audit Auditor) *DefaultGateway {
	return &DefaultGateway{
		authenticator: auth,
		authorizer:    authz,
		auditor:       audit,
	}
}

// Authenticate delegates to the configured Authenticator.
func (g *DefaultGateway) Authenticate(ctx context.Context, creds Credentials) (*Identity, error) {
	return g.authenticator.Authenticate(ctx, creds)
}

// Authorize delegates to the configured Authorizer.
func (g *DefaultGateway) Authorize(ctx context.Context, identity *Identity, action Action, resource Resource) error {
	return g.authorizer.Authorize(ctx, identity, action, resource)
}

// RecordAccess delegates to the configured Auditor.
func (g *DefaultGateway) RecordAccess(ctx context.Context, entry *AuditEntry) error {
	return g.auditor.RecordAccess(ctx, entry)
}
