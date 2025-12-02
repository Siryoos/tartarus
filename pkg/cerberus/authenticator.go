package cerberus

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
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

// SignedAPIKeyAuthenticator validates JWT-based API keys.
type SignedAPIKeyAuthenticator struct {
	secretProvider SecretProvider
}

// NewSignedAPIKeyAuthenticator creates a new authenticator that validates signed API keys.
func NewSignedAPIKeyAuthenticator(secretProvider SecretProvider) *SignedAPIKeyAuthenticator {
	return &SignedAPIKeyAuthenticator{
		secretProvider: secretProvider,
	}
}

// Authenticate validates a signed API key credential.
func (a *SignedAPIKeyAuthenticator) Authenticate(ctx context.Context, creds Credentials) (*Identity, error) {
	apiKeyCred, ok := creds.(*APIKeyCredential)
	if !ok {
		return nil, NewAuthenticationError("invalid credential type, expected API key", nil)
	}

	// Parse the JWS
	object, err := jose.ParseSigned(apiKeyCred.Secret, []jose.SignatureAlgorithm{jose.HS256, jose.RS256, jose.ES256})
	if err != nil {
		return nil, NewAuthenticationError("invalid API key format", err)
	}

	// We expect at least one signature
	if len(object.Signatures) == 0 {
		return nil, NewAuthenticationError("API key has no signature", nil)
	}

	// Get the Key ID from the header
	kid := object.Signatures[0].Header.KeyID
	if kid == "" {
		return nil, NewAuthenticationError("API key missing key ID (kid)", nil)
	}

	// Resolve the key
	// We assume the secret provider returns the raw key material (e.g. shared secret or public key PEM)
	// For simplicity, let's assume it returns a shared secret string for HMAC
	// or we might need to parse it if it's a public key.
	// Let's assume it's a shared secret for HS256 for now, or we can try to parse it.
	keyStr, err := a.secretProvider.Resolve(ctx, "key:"+kid)
	if err != nil {
		return nil, NewAuthenticationError(fmt.Sprintf("unknown key ID: %s", kid), err)
	}

	// Verify the signature
	// Note: In a real system, we'd handle different key types (RSA, ECDSA, HMAC).
	// Here we assume HMAC with the resolved secret.
	payload, err := object.Verify([]byte(keyStr))
	if err != nil {
		return nil, NewAuthenticationError("invalid API key signature", err)
	}

	// Parse the payload into an Identity
	var identity Identity
	if err := json.Unmarshal(payload, &identity); err != nil {
		return nil, NewAuthenticationError("invalid API key payload", err)
	}

	// Check expiration
	if !identity.ExpiresAt.IsZero() && time.Now().After(identity.ExpiresAt) {
		return nil, NewAuthenticationError("API key has expired", nil)
	}

	// Update AuthTime
	identity.AuthTime = time.Now()

	return &identity, nil
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
