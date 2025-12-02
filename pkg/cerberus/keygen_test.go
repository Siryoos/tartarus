package cerberus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateHMACSecret(t *testing.T) {
	secret, err := GenerateHMACSecret()
	require.NoError(t, err)
	assert.NotEmpty(t, secret)
	assert.GreaterOrEqual(t, len(secret), 32, "Secret should be at least 32 characters")

	// Generate another and ensure they're different
	secret2, err := GenerateHMACSecret()
	require.NoError(t, err)
	assert.NotEqual(t, secret, secret2, "Secrets should be random and unique")
}

func TestGenerateAPIKey(t *testing.T) {
	secret, err := GenerateHMACSecret()
	require.NoError(t, err)

	identity := &Identity{
		ID:          "service-123",
		Type:        IdentityTypeService,
		TenantID:    "acme-corp",
		DisplayName: "Test Service",
		Roles:       []string{"readonly"},
		Groups:      []string{},
		Attributes:  make(map[string]string),
		AuthTime:    time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}

	token, err := GenerateAPIKey(identity, "key-v1", secret)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Contains(t, token, ".", "Token should be in JWS compact format")

	// Verify the token can be authenticated
	testProvider := &mockSecretProvider{
		secrets: map[string]string{
			"key:key-v1": secret,
		},
	}

	auth := NewSignedAPIKeyAuthenticator(testProvider)
	creds := &APIKeyCredential{
		KeyID:  "key-v1",
		Secret: token,
	}

	verified, err := auth.Authenticate(context.Background(), creds)
	require.NoError(t, err)
	assert.Equal(t, identity.ID, verified.ID)
	assert.Equal(t, identity.TenantID, verified.TenantID)
	assert.Equal(t, identity.DisplayName, verified.DisplayName)
	assert.Equal(t, identity.Roles, verified.Roles)
}

func TestGenerateAPIKey_Validation(t *testing.T) {
	secret, _ := GenerateHMACSecret()

	t.Run("nil identity", func(t *testing.T) {
		_, err := GenerateAPIKey(nil, "key-1", secret)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "identity cannot be nil")
	})

	t.Run("empty key ID", func(t *testing.T) {
		identity := &Identity{ID: "test"}
		_, err := GenerateAPIKey(identity, "", secret)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "keyID cannot be empty")
	})

	t.Run("short secret", func(t *testing.T) {
		identity := &Identity{ID: "test"}
		_, err := GenerateAPIKey(identity, "key-1", "short")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least 32 bytes")
	})
}

func TestRotateAPIKey(t *testing.T) {
	oldSecret, err := GenerateHMACSecret()
	require.NoError(t, err)

	newSecret, err := GenerateHMACSecret()
	require.NoError(t, err)

	identity := &Identity{
		ID:          "service-123",
		Type:        IdentityTypeService,
		TenantID:    "acme-corp",
		DisplayName: "Test Service",
		Roles:       []string{"readonly"},
		Groups:      []string{},
		Attributes:  make(map[string]string),
		AuthTime:    time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}

	// Generate old token
	oldToken, err := GenerateAPIKey(identity, "key-v1", oldSecret)
	require.NoError(t, err)

	// Rotate to new key
	extendExpiry := 90 * 24 * time.Hour
	newToken, err := RotateAPIKey(oldToken, "key-v1", oldSecret, "key-v2", newSecret, &extendExpiry)
	require.NoError(t, err)
	assert.NotEmpty(t, newToken)
	assert.NotEqual(t, oldToken, newToken, "New token should be different")

	// Verify new token works
	testProvider := &mockSecretProvider{
		secrets: map[string]string{
			"key:key-v2": newSecret,
		},
	}
	auth := NewSignedAPIKeyAuthenticator(testProvider)
	creds := &APIKeyCredential{
		KeyID:  "key-v2",
		Secret: newToken,
	}

	verified, err := auth.Authenticate(context.Background(), creds)
	require.NoError(t, err)
	assert.Equal(t, identity.ID, verified.ID)

	// Check that expiry was extended
	info, err := InspectAPIKey(newToken)
	require.NoError(t, err)
	assert.True(t, info.ExpiresAt.After(time.Now().Add(80*24*time.Hour)),
		"Expiry should be extended to ~90 days")
}

func TestInspectAPIKey(t *testing.T) {
	secret, err := GenerateHMACSecret()
	require.NoError(t, err)

	identity := &Identity{
		ID:          "service-123",
		Type:        IdentityTypeService,
		TenantID:    "acme-corp",
		DisplayName: "Test Service",
		Roles:       []string{"admin", "operator"},
		AuthTime:    time.Now(),
		ExpiresAt:   time.Now().Add(30 * 24 * time.Hour),
	}

	token, err := GenerateAPIKey(identity, "key-v1", secret)
	require.NoError(t, err)

	// Inspect the token
	info, err := InspectAPIKey(token)
	require.NoError(t, err)

	assert.Equal(t, "key-v1", info.KeyID)
	assert.Equal(t, identity.ID, info.IdentityID)
	assert.Equal(t, identity.TenantID, info.TenantID)
	assert.Equal(t, identity.DisplayName, info.DisplayName)
	assert.Equal(t, identity.Roles, info.Roles)
	assert.False(t, info.ExpiresAt.IsZero())
}

// mockSecretProvider is a test helper
type mockSecretProvider struct {
	secrets map[string]string
}

func (m *mockSecretProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if val, ok := m.secrets[ref]; ok {
		return val, nil
	}
	return "", nil
}
