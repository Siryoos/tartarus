package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/cerberus"
)

func TestCerberusAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURL := os.Getenv("OLYMPUS_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	adminKey := os.Getenv("TARTARUS_API_KEY")
	if adminKey == "" {
		adminKey = "admin-secret" // Default for testing
	}

	t.Run("Unauthorized Access", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/sandboxes")
		if err != nil {
			t.Fatalf("Server not reachable at %s: %v", baseURL, err)
		}
		defer resp.Body.Close()

		// Expect 401 Unauthorized
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Expected 401 for missing auth header")
	})

	t.Run("Invalid API Key", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/sandboxes", nil)
		req.Header.Set("Authorization", "Bearer invalid-key")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Expected 401 for invalid key")
	})

	t.Run("Valid Simple API Key", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/sandboxes", nil)
		req.Header.Set("Authorization", "Bearer "+adminKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 for valid admin key")

		// Verify we can parse the response
		var sandboxes []map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&sandboxes)
		assert.NoError(t, err)
	})

	t.Run("Signed API Key Authentication", func(t *testing.T) {
		// Skip if signing keys are not configured
		signingKey := os.Getenv("CERBERUS_KEY_test")
		if signingKey == "" {
			t.Skip("Skipping signed API key test: CERBERUS_KEY_test not configured")
		}

		// Generate a signed API key
		identity := &cerberus.Identity{
			ID:          "test-service",
			Type:        cerberus.IdentityTypeService,
			TenantID:    "default",
			DisplayName: "Test Service",
			Roles:       []string{"readonly"},
			Groups:      []string{},
			Attributes:  make(map[string]string),
			AuthTime:    time.Now(),
			ExpiresAt:   time.Now().Add(1 * time.Hour),
		}

		token, err := cerberus.GenerateAPIKey(identity, "test", signingKey)
		require.NoError(t, err)

		// Use the signed API key
		req, _ := http.NewRequest("GET", baseURL+"/sandboxes", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 for valid signed API key")
	})

	t.Run("mTLS Authentication", func(t *testing.T) {
		// Skip if mTLS is not configured
		if os.Getenv("TLS_CLIENT_AUTH") != "require-verify" {
			t.Skip("Skipping mTLS test: TLS_CLIENT_AUTH not set to require-verify")
		}

		// Generate CA and client certificate for testing
		ca, caKey := generateCA(t)
		clientCert, clientKey := generateClientCert(t, ca, caKey, "test-agent")

		// Create TLS config with client certificate
		tlsCert := tls.Certificate{
			Certificate: [][]byte{clientCert.Raw},
			PrivateKey:  clientKey,
		}

		tlsConfig := &tls.Config{
			Certificates:       []tls.Certificate{tlsCert},
			InsecureSkipVerify: true, // For testing only
		}

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}

		req, _ := http.NewRequest("GET", baseURL+"/sandboxes", nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should be authenticated via mTLS
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 for valid mTLS cert")
	})
}

// Helper functions for certificate generation
func generateCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certBytes)
	require.NoError(t, err)

	return cert, key
}

func generateClientCert(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, cn string) (*x509.Certificate, *rsa.PrivateKey) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"agents"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certBytes)
	require.NoError(t, err)

	return cert, key
}

// TestCerberusRBAC tests role-based access control enforcement
func TestCerberusRBAC(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURL := os.Getenv("OLYMPUS_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	// Skip if RBAC policies are not configured
	if os.Getenv("RBAC_POLICY_PATH") == "" {
		t.Skip("Skipping RBAC test: RBAC_POLICY_PATH not configured")
	}

	signingKey := os.Getenv("CERBERUS_KEY_test")
	if signingKey == "" {
		t.Skip("Skipping RBAC test: CERBERUS_KEY_test not configured")
	}

	t.Run("Readonly User Cannot Create", func(t *testing.T) {
		// Generate readonly user token
		identity := &cerberus.Identity{
			ID:          "readonly-user",
			Type:        cerberus.IdentityTypeUser,
			TenantID:    "default",
			DisplayName: "Readonly User",
			Roles:       []string{"readonly"},
			AuthTime:    time.Now(),
			ExpiresAt:   time.Now().Add(1 * time.Hour),
		}

		token, err := cerberus.GenerateAPIKey(identity, "test", signingKey)
		require.NoError(t, err)

		// Try to create a sandbox (should be denied)
		req, _ := http.NewRequest("POST", baseURL+"/submit", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should be forbidden
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Expected 403 for readonly user creating sandbox")
	})

	t.Run("Admin User Can Create", func(t *testing.T) {
		// Generate admin user token
		identity := &cerberus.Identity{
			ID:          "admin-user",
			Type:        cerberus.IdentityTypeUser,
			TenantID:    "default",
			DisplayName: "Admin User",
			Roles:       []string{"admin"},
			AuthTime:    time.Now(),
			ExpiresAt:   time.Now().Add(1 * time.Hour),
		}

		token, err := cerberus.GenerateAPIKey(identity, "test", signingKey)
		require.NoError(t, err)

		// Try to create a sandbox (should succeed or fail with different error)
		req, _ := http.NewRequest("POST", baseURL+"/submit", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should NOT be forbidden (might be 400 bad request if body is invalid, but not 403)
		assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "Admin user should not be forbidden")
	})
}

// TestAPIKeyRotation tests the key rotation flow
func TestAPIKeyRotation(t *testing.T) {
	oldSecret, err := cerberus.GenerateHMACSecret()
	require.NoError(t, err)

	newSecret, err := cerberus.GenerateHMACSecret()
	require.NoError(t, err)

	// Create original identity and token
	identity := &cerberus.Identity{
		ID:          "service-123",
		Type:        cerberus.IdentityTypeService,
		TenantID:    "acme-corp",
		DisplayName: "Test Service",
		Roles:       []string{"operator"},
		AuthTime:    time.Now(),
		ExpiresAt:   time.Now().Add(30 * 24 * time.Hour),
	}

	oldToken, err := cerberus.GenerateAPIKey(identity, "key-v1", oldSecret)
	require.NoError(t, err)

	// Rotate the key
	extendExpiry := 90 * 24 * time.Hour
	newToken, err := cerberus.RotateAPIKey(oldToken, "key-v1", oldSecret, "key-v2", newSecret, &extendExpiry)
	require.NoError(t, err)

	// Verify both tokens work during grace period
	provider := &mockSecretProvider{
		secrets: map[string]string{
			"key:key-v1": oldSecret,
			"key:key-v2": newSecret,
		},
	}

	auth := cerberus.NewSignedAPIKeyAuthenticator(provider)

	// Old token should still work
	oldCreds := &cerberus.APIKeyCredential{Secret: oldToken}
	oldIdentity, err := auth.Authenticate(context.Background(), oldCreds)
	require.NoError(t, err)
	assert.Equal(t, identity.ID, oldIdentity.ID)

	// New token should work
	newCreds := &cerberus.APIKeyCredential{Secret: newToken}
	newIdentity, err := auth.Authenticate(context.Background(), newCreds)
	require.NoError(t, err)
	assert.Equal(t, identity.ID, newIdentity.ID)

	// New token should have extended expiry
	info, err := cerberus.InspectAPIKey(newToken)
	require.NoError(t, err)
	assert.True(t, info.ExpiresAt.After(time.Now().Add(80*24*time.Hour)))
}

type mockSecretProvider struct {
	secrets map[string]string
}

func (m *mockSecretProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if val, ok := m.secrets[ref]; ok {
		return val, nil
	}
	return "", nil
}
