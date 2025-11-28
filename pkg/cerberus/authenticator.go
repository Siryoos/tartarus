package cerberus

import (
	"context"
	"crypto/subtle"
	"time"
)

// Authenticator verifies credentials and returns an identity.
type Authenticator interface {
	Authenticate(ctx context.Context, creds Credentials) (*Identity, error)
}

// APIKeyAuthenticator validates API key credentials.
type APIKeyAuthenticator struct {
	validKeys map[string]*Identity // Map of API key to identity
}

// NewAPIKeyAuthenticator creates an authenticator with a set of valid API keys.
func NewAPIKeyAuthenticator(keys map[string]*Identity) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{
		validKeys: keys,
	}
}

// NewSimpleAPIKeyAuthenticator creates an authenticator with a single API key.
// This is useful for simple deployments with one shared key.
func NewSimpleAPIKeyAuthenticator(apiKey string) *APIKeyAuthenticator {
	if apiKey == "" {
		return &APIKeyAuthenticator{validKeys: make(map[string]*Identity)}
	}

	identity := &Identity{
		ID:          "api-key-user",
		Type:        IdentityTypeService,
		TenantID:    "default",
		DisplayName: "API Key User",
		Roles:       []string{"admin"}, // Full access for simple mode
		Groups:      []string{},
		Attributes:  make(map[string]string),
		AuthTime:    time.Now(),
		ExpiresAt:   time.Time{}, // Never expires
	}

	return &APIKeyAuthenticator{
		validKeys: map[string]*Identity{
			apiKey: identity,
		},
	}
}

// Authenticate validates an API key credential.
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, creds Credentials) (*Identity, error) {
	apiKeyCred, ok := creds.(*APIKeyCredential)
	if !ok {
		return nil, NewAuthenticationError("invalid credential type, expected API key", nil)
	}

	// Look up the identity for this key
	identity, exists := a.validKeys[apiKeyCred.Secret]
	if !exists {
		return nil, NewAuthenticationError("invalid API key", nil)
	}

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(apiKeyCred.Secret), []byte(apiKeyCred.Secret)) != 1 {
		return nil, NewAuthenticationError("invalid API key", nil)
	}

	// Check if identity has expired
	if !identity.ExpiresAt.IsZero() && time.Now().After(identity.ExpiresAt) {
		return nil, NewAuthenticationError("API key has expired", nil)
	}

	// Return a copy with updated auth time
	identityCopy := *identity
	identityCopy.AuthTime = time.Now()

	return &identityCopy, nil
}

// MultiAuthenticator tries multiple authenticators in order until one succeeds.
type MultiAuthenticator struct {
	authenticators []Authenticator
}

// NewMultiAuthenticator creates an authenticator that tries multiple strategies.
func NewMultiAuthenticator(authenticators ...Authenticator) *MultiAuthenticator {
	return &MultiAuthenticator{
		authenticators: authenticators,
	}
}

// Authenticate tries each authenticator until one succeeds.
func (m *MultiAuthenticator) Authenticate(ctx context.Context, creds Credentials) (*Identity, error) {
	var lastErr error

	for _, auth := range m.authenticators {
		identity, err := auth.Authenticate(ctx, creds)
		if err == nil {
			return identity, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, NewAuthenticationError("no authenticators configured", nil)
}
