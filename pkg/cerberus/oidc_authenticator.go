package cerberus

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCAuthenticator validates OIDC ID tokens.
type OIDCAuthenticator struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	clientID string
}

// NewOIDCAuthenticator creates a new OIDC authenticator.
// It discovers the provider configuration from the issuer URL.
func NewOIDCAuthenticator(ctx context.Context, issuerURL, clientID string) (*OIDCAuthenticator, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: clientID,
	})

	return &OIDCAuthenticator{
		provider: provider,
		verifier: verifier,
		clientID: clientID,
	}, nil
}

// Authenticate validates the OIDC ID token.
// The credential must be a BearerTokenCredential.
func (a *OIDCAuthenticator) Authenticate(ctx context.Context, creds Credentials) (*Identity, error) {
	tokenCred, ok := creds.(*BearerTokenCredential)
	if !ok {
		return nil, NewAuthenticationError("invalid credential type, expected bearer token", nil)
	}

	idToken, err := a.verifier.Verify(ctx, tokenCred.Token)
	if err != nil {
		return nil, NewAuthenticationError("invalid token", err)
	}

	// Extract claims
	var claims struct {
		Subject       string   `json:"sub"`
		Email         string   `json:"email"`
		EmailVerified bool     `json:"email_verified"`
		Groups        []string `json:"groups"`
		Name          string   `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, NewAuthenticationError("failed to parse claims", err)
	}

	// Map to Identity
	identity := &Identity{
		ID:          claims.Subject,
		Type:        IdentityTypeUser,
		TenantID:    "default", // Could be mapped from a claim if needed
		DisplayName: claims.Name,
		Roles:       []string{}, // Roles are typically assigned via RBAC binding, but we can map groups to roles here if simple
		Groups:      claims.Groups,
		Attributes: map[string]string{
			"email": claims.Email,
		},
		AuthTime:  idToken.IssuedAt,
		ExpiresAt: idToken.Expiry,
	}

	if identity.DisplayName == "" {
		identity.DisplayName = claims.Email
	}

	return identity, nil
}

// BearerTokenCredential holds a raw bearer token.
type BearerTokenCredential struct {
	Token string
}

// Type returns the type of the credential.
func (c *BearerTokenCredential) Type() CredentialType {
	return CredentialTypeOAuth2
}
