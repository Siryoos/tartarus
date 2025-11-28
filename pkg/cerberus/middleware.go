package cerberus

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	// IdentityContextKey is the context key for the authenticated identity.
	IdentityContextKey contextKey = "cerberus.identity"
)

// HTTPMiddleware wraps an HTTP handler with Cerberus authentication, authorization, and audit.
type HTTPMiddleware struct {
	gateway   Gateway
	extractor CredentialExtractor
	mapper    ResourceMapper
}

// CredentialExtractor extracts credentials from an HTTP request.
type CredentialExtractor interface {
	Extract(r *http.Request) (Credentials, error)
}

// ResourceMapper maps an HTTP request to a Cerberus resource and action.
type ResourceMapper interface {
	MapRequest(r *http.Request) (Action, Resource, error)
}

// NewHTTPMiddleware creates middleware that enforces Cerberus security.
func NewHTTPMiddleware(gateway Gateway, extractor CredentialExtractor, mapper ResourceMapper) *HTTPMiddleware {
	return &HTTPMiddleware{
		gateway:   gateway,
		extractor: extractor,
		mapper:    mapper,
	}
}

// Wrap returns an HTTP handler that enforces authentication, authorization, and audit.
func (m *HTTPMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Extract credentials from request
		creds, err := m.extractor.Extract(r)
		if err != nil {
			m.recordAndRespond(r.Context(), w, r, nil, AuditResultDenied, err, startTime)
			http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// Authenticate
		identity, err := m.gateway.Authenticate(r.Context(), creds)
		if err != nil {
			m.recordAndRespond(r.Context(), w, r, nil, AuditResultDenied, err, startTime)
			http.Error(w, "Unauthorized: Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Map request to action and resource
		action, resource, err := m.mapper.MapRequest(r)
		if err != nil {
			m.recordAndRespond(r.Context(), w, r, identity, AuditResultError, err, startTime)
			http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Authorize
		if err := m.gateway.Authorize(r.Context(), identity, action, resource); err != nil {
			m.recordAndRespond(r.Context(), w, r, identity, AuditResultDenied, err, startTime)
			http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
			return
		}

		// Inject identity into context
		ctx := context.WithValue(r.Context(), IdentityContextKey, identity)
		r = r.WithContext(ctx)

		// Record successful access
		m.recordAndRespond(ctx, w, r, identity, AuditResultSuccess, nil, startTime)

		// Continue to next handler
		next.ServeHTTP(w, r)
	})
}

// recordAndRespond creates an audit entry and records it.
func (m *HTTPMiddleware) recordAndRespond(ctx context.Context, w http.ResponseWriter, r *http.Request, identity *Identity, result AuditResult, err error, startTime time.Time) {
	action, resource, _ := m.mapper.MapRequest(r)

	entry := &AuditEntry{
		Timestamp: time.Now(),
		RequestID: r.Header.Get("X-Request-ID"),
		Identity:  identity,
		Action:    action,
		Resource:  resource,
		Result:    result,
		Latency:   time.Since(startTime),
		SourceIP:  getSourceIP(r),
		UserAgent: r.UserAgent(),
	}

	if err != nil {
		entry.ErrorMessage = err.Error()
	}

	// Record audit entry (don't fail request if audit fails)
	_ = m.gateway.RecordAccess(ctx, entry)
}

// BearerTokenExtractor extracts API key credentials from the Authorization header.
type BearerTokenExtractor struct{}

// NewBearerTokenExtractor creates a credential extractor for bearer tokens.
func NewBearerTokenExtractor() *BearerTokenExtractor {
	return &BearerTokenExtractor{}
}

// Extract parses the Authorization header and returns API key credentials.
func (e *BearerTokenExtractor) Extract(r *http.Request) (Credentials, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, NewAuthenticationError("missing Authorization header", nil)
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, NewAuthenticationError("invalid Authorization header format", nil)
	}

	return &APIKeyCredential{
		KeyID:  "", // Not used in simple mode
		Secret: parts[1],
	}, nil
}

// DefaultResourceMapper provides a simple mapping from HTTP requests to resources.
type DefaultResourceMapper struct{}

// NewDefaultResourceMapper creates a basic resource mapper.
func NewDefaultResourceMapper() *DefaultResourceMapper {
	return &DefaultResourceMapper{}
}

// MapRequest maps HTTP method and path to action and resource.
func (m *DefaultResourceMapper) MapRequest(r *http.Request) (Action, Resource, error) {
	// Map HTTP method to action
	var action Action
	switch r.Method {
	case http.MethodGet:
		action = ActionRead
	case http.MethodPost:
		action = ActionCreate
	case http.MethodPut, http.MethodPatch:
		action = ActionUpdate
	case http.MethodDelete:
		action = ActionDelete
	default:
		action = ActionRead
	}

	// Parse path to determine resource type
	// This is a simple implementation; a real one would parse the path more carefully
	var resourceType ResourceType
	var resourceID string

	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/sandboxes"):
		resourceType = ResourceTypeSandbox
		// Extract ID if present: /sandboxes/{id}
		parts := strings.Split(strings.TrimPrefix(path, "/sandboxes/"), "/")
		if len(parts) > 0 && parts[0] != "" {
			resourceID = parts[0]
		}
	case strings.HasPrefix(path, "/templates"):
		resourceType = ResourceTypeTemplate
	case strings.HasPrefix(path, "/policies"):
		resourceType = ResourceTypePolicy
	default:
		resourceType = ResourceTypeSandbox // Default
	}

	resource := Resource{
		Type:      resourceType,
		ID:        resourceID,
		TenantID:  "default", // TODO: Extract from identity or request
		Namespace: "default",
	}

	return action, resource, nil
}

// GetIdentity retrieves the authenticated identity from the request context.
func GetIdentity(ctx context.Context) (*Identity, bool) {
	identity, ok := ctx.Value(IdentityContextKey).(*Identity)
	return identity, ok
}

// getSourceIP extracts the client IP from the request.
func getSourceIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}
