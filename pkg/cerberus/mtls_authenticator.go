package cerberus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"
)

// MTLSAuthenticator validates client certificates.
type MTLSAuthenticator struct {
	// TrustedCAs is the pool of trusted CAs for client certificates.
	// If nil, the system's default root CAs are used (which might not be what we want for mTLS).
	TrustedCAs *x509.CertPool
}

// NewMTLSAuthenticator creates a new mTLS authenticator.
func NewMTLSAuthenticator(trustedCAs *x509.CertPool) *MTLSAuthenticator {
	return &MTLSAuthenticator{
		TrustedCAs: trustedCAs,
	}
}

// Authenticate validates the mTLS credential.
// The credential must be an MTLSCredential.
func (a *MTLSAuthenticator) Authenticate(ctx context.Context, creds Credentials) (*Identity, error) {
	mtlsCred, ok := creds.(*MTLSCredential)
	if !ok {
		return nil, NewAuthenticationError("invalid credential type, expected mTLS", nil)
	}

	if len(mtlsCred.ConnectionState.PeerCertificates) == 0 {
		return nil, NewAuthenticationError("no client certificate provided", nil)
	}

	// The first certificate is the leaf
	cert := mtlsCred.ConnectionState.PeerCertificates[0]

	// Verify the certificate chain
	opts := x509.VerifyOptions{
		Roots:     a.TrustedCAs,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if _, err := cert.Verify(opts); err != nil {
		return nil, NewAuthenticationError("failed to verify client certificate", err)
	}

	// Extract identity from Subject CN or SANs
	// We prioritize SPIFFE ID in URIs if present, otherwise CN
	var id string
	var identityType IdentityType = IdentityTypeAgent // Default to agent for mTLS

	// Check for SPIFFE ID in URIs
	for _, uri := range cert.URIs {
		if uri.Scheme == "spiffe" {
			id = uri.String()
			break
		}
	}

	// Fallback to Common Name
	if id == "" {
		id = cert.Subject.CommonName
	}

	if id == "" {
		return nil, NewAuthenticationError("client certificate has no identity (CN or SPIFFE URI)", nil)
	}

	// Map to Identity
	identity := &Identity{
		ID:          id,
		Type:        identityType,
		TenantID:    "default",
		DisplayName: cert.Subject.CommonName,
		Roles:       []string{"agent"}, // Default role for mTLS agents
		Groups:      cert.Subject.Organization,
		Attributes: map[string]string{
			"serial_number": cert.SerialNumber.String(),
			"issuer":        cert.Issuer.CommonName,
		},
		AuthTime:  time.Now(),
		ExpiresAt: cert.NotAfter,
	}

	return identity, nil
}

// MTLSCredential holds the TLS connection state.
type MTLSCredential struct {
	ConnectionState tls.ConnectionState
}

// Type returns the type of the credential.
func (c *MTLSCredential) Type() CredentialType {
	return CredentialTypeMTLS
}
