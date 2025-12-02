package cerberus

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// GenerateHMACSecret generates a cryptographically secure HMAC secret.
// The secret is 32 bytes (256 bits) suitable for HS256 signing.
func GenerateHMACSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateAPIKey generates a signed JWT-based API key for the given identity.
// The key is signed with HMAC-SHA256 using the provided secret and key ID.
//
// Parameters:
//   - identity: The identity to encode in the token
//   - keyID: The key ID (kid) used to identify the signing key
//   - secret: The HMAC secret used to sign the token (must be at least 32 bytes)
//
// Returns:
//   - A JWS compact serialized token that can be used as an API key
//   - An error if signing fails
//
// Example:
//
//	identity := &Identity{
//	    ID: "service-123",
//	    Type: IdentityTypeService,
//	    TenantID: "acme-corp",
//	    DisplayName: "Analytics Service",
//	    Roles: []string{"readonly"},
//	    ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
//	}
//	token, err := GenerateAPIKey(identity, "key-v1", "your-secret")
func GenerateAPIKey(identity *Identity, keyID string, secret string) (string, error) {
	if identity == nil {
		return "", fmt.Errorf("identity cannot be nil")
	}
	if keyID == "" {
		return "", fmt.Errorf("keyID cannot be empty")
	}
	if len(secret) < 32 {
		return "", fmt.Errorf("secret must be at least 32 bytes long")
	}

	// Create signer with HS256 algorithm and key ID header
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS256, Key: []byte(secret)},
		(&jose.SignerOptions{}).WithHeader("kid", keyID),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create signer: %w", err)
	}

	// Marshal identity to JSON payload
	payload, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("failed to marshal identity: %w", err)
	}

	// Sign the payload
	object, err := signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("failed to sign payload: %w", err)
	}

	// Serialize to compact form (header.payload.signature)
	token, err := object.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize token: %w", err)
	}

	return token, nil
}

// RotateAPIKey generates a new API key with a new key ID while preserving the identity.
// This is useful for implementing key rotation without invalidating the user's access.
//
// The old key should remain valid for a grace period to allow clients to update.
//
// Parameters:
//   - oldToken: The existing API key token
//   - oldKeyID: The key ID of the old signing key
//   - oldSecret: The secret used to verify the old token
//   - newKeyID: The key ID for the new signing key
//   - newSecret: The secret for the new signing key
//   - extendExpiry: Optional duration to extend the expiry time (e.g., 90 days)
//
// Returns:
//   - A new API key token signed with the new key
//   - An error if verification or signing fails
func RotateAPIKey(oldToken, oldKeyID, oldSecret, newKeyID, newSecret string, extendExpiry *time.Duration) (string, error) {
	// Verify and decode the old token
	object, err := jose.ParseSigned(oldToken, []jose.SignatureAlgorithm{jose.HS256})
	if err != nil {
		return "", fmt.Errorf("failed to parse old token: %w", err)
	}

	// Verify with old secret
	payload, err := object.Verify([]byte(oldSecret))
	if err != nil {
		return "", fmt.Errorf("failed to verify old token: %w", err)
	}

	// Decode identity
	var identity Identity
	if err := json.Unmarshal(payload, &identity); err != nil {
		return "", fmt.Errorf("failed to unmarshal identity: %w", err)
	}

	// Extend expiry if requested
	if extendExpiry != nil {
		identity.ExpiresAt = time.Now().Add(*extendExpiry)
	}

	// Generate new token with new key
	return GenerateAPIKey(&identity, newKeyID, newSecret)
}

// APIKeyInfo represents metadata about an API key without the secret.
type APIKeyInfo struct {
	KeyID       string    `json:"key_id"`
	IdentityID  string    `json:"identity_id"`
	TenantID    string    `json:"tenant_id"`
	DisplayName string    `json:"display_name"`
	Roles       []string  `json:"roles"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

// InspectAPIKey extracts metadata from an API key without verifying it.
// This is useful for debugging or displaying key information.
// Note: This does NOT verify the signature, so use with caution.
func InspectAPIKey(token string) (*APIKeyInfo, error) {
	object, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.HS256, jose.RS256, jose.ES256})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if len(object.Signatures) == 0 {
		return nil, fmt.Errorf("token has no signatures")
	}

	// Get key ID from header
	keyID := object.Signatures[0].Header.KeyID

	// Decode payload WITHOUT verification (just for inspection)
	// This is safe because we're only reading metadata, not trusting it
	var identity Identity
	if err := json.Unmarshal(object.UnsafePayloadWithoutVerification(), &identity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal identity: %w", err)
	}

	return &APIKeyInfo{
		KeyID:       keyID,
		IdentityID:  identity.ID,
		TenantID:    identity.TenantID,
		DisplayName: identity.DisplayName,
		Roles:       identity.Roles,
		IssuedAt:    identity.AuthTime,
		ExpiresAt:   identity.ExpiresAt,
	}, nil
}
