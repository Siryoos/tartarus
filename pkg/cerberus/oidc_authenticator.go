package cerberus

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCAuthenticator validates OIDC ID tokens and Access Tokens.
type OIDCAuthenticator struct {
	provider            *oidc.Provider
	idTokenVerifier     *oidc.IDTokenVerifier
	accessTokenVerifier *oidc.IDTokenVerifier
	clientID            string
}

// NewOIDCAuthenticator creates a new OIDC authenticator.
// It discovers the provider configuration from the issuer URL.
// apiAudience is optional; if provided, it enables validation of Access Tokens (Client Credentials flow)
// with that specific audience.
func NewOIDCAuthenticator(ctx context.Context, issuerURL, clientID string, apiAudience string) (*OIDCAuthenticator, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider: %w", err)
	}

	idTokenVerifier := provider.Verifier(&oidc.Config{
		ClientID: clientID,
	})

	var accessTokenVerifier *oidc.IDTokenVerifier
	if apiAudience != "" {
		accessTokenVerifier = provider.Verifier(&oidc.Config{
			ClientID: apiAudience,
		})
	}

	return &OIDCAuthenticator{
		provider:            provider,
		idTokenVerifier:     idTokenVerifier,
		accessTokenVerifier: accessTokenVerifier,
		clientID:            clientID,
	}, nil
}

// Authenticate validates the OIDC ID token or Access Token.
// The credential must be a BearerTokenCredential.
func (a *OIDCAuthenticator) Authenticate(ctx context.Context, creds Credentials) (*Identity, error) {
	tokenCred, ok := creds.(*BearerTokenCredential)
	if !ok {
		return nil, NewAuthenticationError("invalid credential type, expected bearer token", nil)
	}

	// Try verifying as ID Token (User Flow)
	idToken, err := a.idTokenVerifier.Verify(ctx, tokenCred.Token)
	if err == nil {
		return a.identityFromToken(idToken, IdentityTypeUser)
	}

	// If failed, and we have an access token verifier, try that (Service Flow)
	if a.accessTokenVerifier != nil {
		accessToken, err := a.accessTokenVerifier.Verify(ctx, tokenCred.Token)
		if err == nil {
			return a.identityFromToken(accessToken, IdentityTypeService)
		}
	}

	return nil, NewAuthenticationError("invalid token", err)
}

func (a *OIDCAuthenticator) identityFromToken(token *oidc.IDToken, idType IdentityType) (*Identity, error) {
	// Extract claims
	var claims struct {
		Subject       string   `json:"sub"`
		Email         string   `json:"email"`
		EmailVerified bool     `json:"email_verified"`
		Groups        []string `json:"groups"`
		Name          string   `json:"name"`
		// Service accounts might have 'client_id' or 'azp' as subject or separate claim
		ClientID string `json:"client_id"`
	}
	if err := token.Claims(&claims); err != nil {
		return nil, NewAuthenticationError("failed to parse claims", err)
	}

	// For service accounts, Subject might be the client ID or a UUID.
	// If Name is empty, use ClientID or Subject.
	displayName := claims.Name
	if displayName == "" {
		if claims.ClientID != "" {
			displayName = claims.ClientID
		} else {
			displayName = claims.Subject
		}
	}

	// Map to Identity
	identity := &Identity{
		ID:          claims.Subject,
		Type:        idType,
		TenantID:    "default", // Could be mapped from a claim if needed
		DisplayName: displayName,
		Roles:       []string{}, // Roles are typically assigned via RBAC binding, but we can map groups to roles here if simple
		Groups:      claims.Groups,
		Attributes: map[string]string{
			"email": claims.Email,
		},
		AuthTime:  token.IssuedAt,
		ExpiresAt: token.Expiry,
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
