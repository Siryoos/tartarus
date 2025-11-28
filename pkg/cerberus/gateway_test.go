package cerberus

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
)

func TestGateway(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Setup components
	auth := NewSimpleAPIKeyAuthenticator("test-key")
	authz := NewAllowAllAuthorizer()
	audit := NewLogAuditor(logger)

	gateway := NewGateway(auth, authz, audit)

	// Test authentication
	creds := &APIKeyCredential{Secret: "test-key"}
	identity, err := gateway.Authenticate(ctx, creds)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	if identity.ID != "api-key-user" {
		t.Errorf("got identity ID %s, want api-key-user", identity.ID)
	}

	// Test authorization
	resource := Resource{
		Type: ResourceTypeSandbox,
		ID:   "sandbox-123",
	}

	err = gateway.Authorize(ctx, identity, ActionCreate, resource)
	if err != nil {
		t.Errorf("authorization failed: %v", err)
	}

	// Test audit
	entry := &AuditEntry{
		RequestID: "req-123",
		Identity:  identity,
		Action:    ActionCreate,
		Resource:  resource,
		Result:    AuditResultSuccess,
	}

	err = gateway.RecordAccess(ctx, entry)
	if err != nil {
		t.Errorf("audit failed: %v", err)
	}
}

func TestGateway_AuthenticationFailure(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	auth := NewSimpleAPIKeyAuthenticator("correct-key")
	authz := NewAllowAllAuthorizer()
	audit := NewLogAuditor(logger)

	gateway := NewGateway(auth, authz, audit)

	// Try with wrong key
	creds := &APIKeyCredential{Secret: "wrong-key"}
	_, err := gateway.Authenticate(ctx, creds)
	if err == nil {
		t.Error("expected authentication error, got nil")
	}

	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Errorf("expected AuthenticationError, got %T", err)
	}
}

func TestGateway_AuthorizationFailure(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	auth := NewSimpleAPIKeyAuthenticator("test-key")
	// Use deny-all authorizer
	authz := NewDenyAllAuthorizer()
	audit := NewLogAuditor(logger)

	gateway := NewGateway(auth, authz, audit)

	identity := &Identity{
		ID:   "test-user",
		Type: IdentityTypeUser,
	}

	resource := Resource{
		Type: ResourceTypeSandbox,
		ID:   "sandbox-123",
	}

	err := gateway.Authorize(ctx, identity, ActionCreate, resource)
	if err == nil {
		t.Error("expected authorization error, got nil")
	}

	var authzErr *AuthorizationError
	if !errors.As(err, &authzErr) {
		t.Errorf("expected AuthorizationError, got %T", err)
	}
}
