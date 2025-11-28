package cerberus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAPIKeyAuthenticator(t *testing.T) {
	ctx := context.Background()

	identity := &Identity{
		ID:          "test-user",
		Type:        IdentityTypeUser,
		TenantID:    "test-tenant",
		DisplayName: "Test User",
		Roles:       []string{"user"},
		Groups:      []string{"developers"},
		Attributes:  make(map[string]string),
		ExpiresAt:   time.Time{}, // Never expires
	}

	auth := NewAPIKeyAuthenticator(map[string]*Identity{
		"valid-key": identity,
	})

	tests := []struct {
		name    string
		creds   Credentials
		wantErr bool
		wantID  string
	}{
		{
			name: "valid API key",
			creds: &APIKeyCredential{
				Secret: "valid-key",
			},
			wantErr: false,
			wantID:  "test-user",
		},
		{
			name: "invalid API key",
			creds: &APIKeyCredential{
				Secret: "invalid-key",
			},
			wantErr: true,
		},
		{
			name:    "wrong credential type",
			creds:   &OAuth2Credential{AccessToken: "token"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := auth.Authenticate(ctx, tt.creds)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.ID != tt.wantID {
				t.Errorf("got identity ID %s, want %s", result.ID, tt.wantID)
			}
		})
	}
}

func TestSimpleAPIKeyAuthenticator(t *testing.T) {
	ctx := context.Background()

	auth := NewSimpleAPIKeyAuthenticator("secret-key")

	// Valid key
	result, err := auth.Authenticate(ctx, &APIKeyCredential{Secret: "secret-key"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ID != "api-key-user" {
		t.Errorf("got identity ID %s, want api-key-user", result.ID)
	}

	// Invalid key
	_, err = auth.Authenticate(ctx, &APIKeyCredential{Secret: "wrong-key"})
	if err == nil {
		t.Error("expected error for invalid key, got nil")
	}
}

func TestAPIKeyAuthenticator_ExpiredKey(t *testing.T) {
	ctx := context.Background()

	expiredIdentity := &Identity{
		ID:        "expired-user",
		Type:      IdentityTypeUser,
		TenantID:  "test-tenant",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}

	auth := NewAPIKeyAuthenticator(map[string]*Identity{
		"expired-key": expiredIdentity,
	})

	_, err := auth.Authenticate(ctx, &APIKeyCredential{Secret: "expired-key"})
	if err == nil {
		t.Error("expected error for expired key, got nil")
	}

	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Errorf("expected AuthenticationError, got %T", err)
	}
}

func TestMultiAuthenticator(t *testing.T) {
	ctx := context.Background()

	identity1 := &Identity{ID: "user1", Type: IdentityTypeUser}
	identity2 := &Identity{ID: "user2", Type: IdentityTypeService}

	auth1 := NewAPIKeyAuthenticator(map[string]*Identity{"key1": identity1})
	auth2 := NewAPIKeyAuthenticator(map[string]*Identity{"key2": identity2})

	multi := NewMultiAuthenticator(auth1, auth2)

	// Should succeed with first authenticator
	result, err := multi.Authenticate(ctx, &APIKeyCredential{Secret: "key1"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ID != "user1" {
		t.Errorf("got identity ID %s, want user1", result.ID)
	}

	// Should succeed with second authenticator
	result, err = multi.Authenticate(ctx, &APIKeyCredential{Secret: "key2"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ID != "user2" {
		t.Errorf("got identity ID %s, want user2", result.ID)
	}

	// Should fail with invalid key
	_, err = multi.Authenticate(ctx, &APIKeyCredential{Secret: "invalid"})
	if err == nil {
		t.Error("expected error for invalid key, got nil")
	}
}
