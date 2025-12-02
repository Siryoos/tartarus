package cerberus_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/cerberus"
)

// MockSecretProvider implements cerberus.SecretProvider
type MockSecretProvider struct {
	Secrets map[string]string
}

func (m *MockSecretProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if val, ok := m.Secrets[ref]; ok {
		return val, nil
	}
	return "", nil
}

func TestSignedAPIKeyAuthenticator(t *testing.T) {
	// Generate a signing key (HMAC) - must be at least 32 bytes for HS256
	secret := "my-super-secret-key-123-must-be-32-bytes-long"
	keyID := "key-1"

	// Create a JWS
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte(secret)}, (&jose.SignerOptions{}).WithHeader("kid", keyID))
	require.NoError(t, err)

	identity := cerberus.Identity{
		ID:          "user-123",
		Type:        cerberus.IdentityTypeUser,
		DisplayName: "Test User",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	payload, err := json.Marshal(identity)
	require.NoError(t, err)

	object, err := signer.Sign(payload)
	require.NoError(t, err)

	token, err := object.CompactSerialize()
	require.NoError(t, err)

	// Setup Authenticator
	provider := &MockSecretProvider{
		Secrets: map[string]string{
			"key:" + keyID: secret,
		},
	}
	auth := cerberus.NewSignedAPIKeyAuthenticator(provider)

	// Test Authenticate
	creds := &cerberus.APIKeyCredential{
		Secret: token,
	}

	id, err := auth.Authenticate(context.Background(), creds)
	require.NoError(t, err)
	assert.Equal(t, identity.ID, id.ID)
	assert.Equal(t, identity.DisplayName, id.DisplayName)
}

func TestMTLSAuthenticator(t *testing.T) {
	// Generate CA
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caBytes)
	require.NoError(t, err)

	// Generate Client Cert
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "agent-1", Organization: []string{"agents"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientBytes, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)
	clientCert, err := x509.ParseCertificate(clientBytes)
	require.NoError(t, err)

	// Setup Authenticator
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	auth := cerberus.NewMTLSAuthenticator(roots)

	// Test Authenticate
	creds := &cerberus.MTLSCredential{
		ConnectionState: tls.ConnectionState{
			PeerCertificates:  []*x509.Certificate{clientCert},
			HandshakeComplete: true,
		},
	}

	id, err := auth.Authenticate(context.Background(), creds)
	require.NoError(t, err)
	assert.Equal(t, "agent-1", id.ID)
	assert.Equal(t, "agent-1", id.DisplayName)
	assert.Contains(t, id.Groups, "agents")
}
