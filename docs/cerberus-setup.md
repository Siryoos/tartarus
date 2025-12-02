# Cerberus Authentication Gateway Setup

## Overview

Cerberus is the three-headed guardian of Tartarus, providing authentication, authorization, and audit capabilities. Each "head" represents a distinct authentication method:

1. **API Keys** - Signed JWT-based tokens with rotation support
2. **OAuth2/OIDC** - Integration with identity providers (Auth0, Keycloak, etc.)
3. **Mutual TLS** - Certificate-based authentication for agents with automated rotation

## Quick Start

### Simple API Key (Development)

For development and testing, use a simple API key:

```bash
export TARTARUS_API_KEY="your-secret-key"
go run ./cmd/olympus-api
```

Test the API:
```bash
curl -H "Authorization: Bearer your-secret-key" http://localhost:8080/sandboxes
```

## Authentication Methods

### 1. Signed API Keys (Recommended for Services)

Signed API keys are JWT-based tokens that can be rotated without service interruption.

#### Generate a Signing Secret

```bash
# In a Go program or test:
import "github.com/tartarus-sandbox/tartarus/pkg/cerberus"

secret, _ := cerberus.GenerateHMACSecret()
fmt.Println(secret) // Store securely
```

#### Configure Environment

```bash
# Set the signing key with a key ID
export CERBERUS_KEY_prod="<your-secret-from-above>"
```

#### Generate an API Key

```go
package main

import (
	"fmt"
	"time"
	"github.com/tartarus-sandbox/tartarus/pkg/cerberus"
)

func main() {
	identity := &cerberus.Identity{
		ID:          "analytics-service",
		Type:        cerberus.IdentityTypeService,
		TenantID:    "acme-corp",
		DisplayName: "Analytics Service",
		Roles:       []string{"readonly"},
		ExpiresAt:   time.Now().Add(90 * 24 * time.Hour), // 90 days
	}

	token, err := cerberus.GenerateAPIKey(identity, "prod", "<signing-secret>")
	if err != nil {
		panic(err)
	}

	fmt.Println("API Key:", token)
	// Give this token to the service
}
```

#### Key Rotation

```go
// Rotate from key-v1 to key-v2
extendExpiry := 90 * 24 * time.Hour
newToken, err := cerberus.RotateAPIKey(
	oldToken,
	"key-v1", oldSecret,
	"key-v2", newSecret,
	&extendExpiry,
)

// Both old and new tokens work during grace period
// Update CERBERUS_KEY_v1 and CERBERUS_KEY_v2 in environment
```

### 2. OAuth2/OIDC (Recommended for Users)

Integrate with your identity provider for user authentication.

#### Configure OIDC

```bash
export OIDC_ISSUER_URL="https://your-domain.auth0.com"
export OIDC_CLIENT_ID="your-client-id"
```

#### Supported Flows

- **Authorization Code Flow** - For user login via browser
- **Client Credentials Flow** - For service-to-service authentication

#### Example: Auth0 Integration

1. Create an Auth0 application
2. Set callback URLs
3. Configure environment:
   ```bash
   export OIDC_ISSUER_URL="https://dev-xyz.auth0.com"
   export OIDC_CLIENT_ID="your-app-client-id"
   ```

#### Using OIDC Tokens

```bash
# Get an access token from your OIDC provider
# Then use it to call Tartarus API
curl -H "Authorization: Bearer <id-token-or-access-token>" \
     http://localhost:8080/sandboxes
```

### 3. Mutual TLS (Recommended for Agents)

Agent communication is secured with mutual TLS and automated certificate rotation.

#### Generate Certificates

```bash
# Generate CA
openssl req -x509 -new -nodes -sha256 -days 3650 \
  -newkey rsa:4096 \
  -keyout ca.key \
  -out ca.crt \
  -subj "/CN=Tartarus CA"

# Generate server certificate
openssl req -new -nodes \
  -newkey rsa:2048 \
  -keyout server.key \
  -out server.csr \
  -subj "/CN=olympus.tartarus.local"

openssl x509 -req -sha256 -days 365 \
  -in server.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt

# Generate agent/client certificate
openssl req -new -nodes \
  -newkey rsa:2048 \
  -keyout agent.key \
  -out agent.csr \
  -subj "/CN=agent-1/O=agents"

openssl x509 -req -sha256 -days 365 \
  -in agent.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -extfile <(echo "extendedKeyUsage=clientAuth") \
  -out agent.crt
```

#### Configure mTLS

```bash
export TLS_CERT_FILE="./certs/server.crt"
export TLS_KEY_FILE="./certs/server.key"
export TLS_CA_FILE="./certs/ca.crt"
export TLS_CLIENT_AUTH="require-verify"
```

#### Test mTLS

```bash
curl --cert ./certs/agent.crt \
     --key ./certs/agent.key \
     --cacert ./certs/ca.crt \
     https://localhost:8080/sandboxes
```

#### Automated Certificate Rotation

Cerberus automatically reloads certificates every 60 seconds. To rotate:

1. Generate new certificates
2. Replace files on disk
3. Cerberus detects changes and reloads
4. Zero downtime

## Role-Based Access Control (RBAC)

### Configure RBAC Policies

Create `rbac-policies.yaml`:

```yaml
admin:
  allowAll: true

operator:
  permissions:
    - actions: [create, read, update, delete]
      resources: [sandbox]
    - actions: [read]
      resources: [template, policy]

readonly:
  permissions:
    - actions: [read]
      resources: [sandbox, template, policy]

agent:
  permissions:
    - actions: [create, read, update]
      resources: [sandbox]
    - actions: [read]
      resources: [template]
```

Enable RBAC:
```bash
export RBAC_POLICY_PATH="./configs/rbac-policies.yaml"
```

### Roles in Identities

Roles are assigned when generating API keys:

```go
identity := &cerberus.Identity{
	ID:    "service-123",
	Roles: []string{"readonly"}, // <-- Role determines permissions
	// ...
}
```

For OIDC, roles can be mapped from groups or claims.

## Secret Providers

### Environment Variables (Default)

```bash
# Simple API key
export TARTARUS_API_KEY="secret"

# Signed API key secrets (with key ID)
export CERBERUS_KEY_prod="secret-for-prod-key"
export CERBERUS_KEY_staging="secret-for-staging-key"
```

### HashiCorp Vault (Production)

Configure Vault secret provider (requires integration code):

```bash
export VAULT_ADDR="https://vault.example.com"
export VAULT_TOKEN="your-vault-token"
```

Secrets are referenced as:
```
vault:secret/tartarus/api-keys:prod
```

### AWS KMS (Production)

Configure KMS secret provider (requires integration code):

```bash
export AWS_REGION="us-east-1"
export KMS_KEY_ARN="arn:aws:kms:us-east-1:123456789:key/abc-123"
```

## Audit Logging

All access attempts are automatically audited with:
- Timestamp
- Identity (user/service/agent)
- Action (create/read/update/delete)
- Resource (sandbox/template/policy)
- Result (success/denied/error)
- Latency
- Source IP

Audit logs are sent to:
1. **Structured logs** (JSON) via slog
2. **Metrics** via Prometheus

Example audit entry:
```json
{
  "timestamp": "2025-12-02T20:30:00Z",
  "identity": {
    "id": "analytics-service",
    "type": "service",
    "tenant_id": "acme-corp"
  },
  "action": "read",
  "resource": {
    "type": "sandbox",
    "id": "sb-123"
  },
  "result": "success",
  "latency_ms": 45,
  "source_ip": "192.168.1.100"
}
```

## Production Checklist

- [ ] Use signed API keys (not simple keys) for services
- [ ] Enable OIDC for user authentication
- [ ] Enable mTLS for agent communication
- [ ] Configure RBAC policies
- [ ] Use Vault or KMS for secret storage
- [ ] Enable TLS (HTTPS) for API endpoints
- [ ] Set up audit log aggregation
- [ ] Configure certificate expiry monitoring
- [ ] Test key rotation procedures
- [ ] Document emergency revocation process

## Environment Variables Reference

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `TARTARUS_API_KEY` | Simple API key | No | - |
| `CERBERUS_KEY_<kid>` | Signing key for API key with ID `<kid>` | No | - |
| `OIDC_ISSUER_URL` | OIDC provider issuer URL | No | - |
| `OIDC_CLIENT_ID` | OIDC client ID | No | - |
| `TLS_CERT_FILE` | Path to server TLS certificate | No | - |
| `TLS_KEY_FILE` | Path to server TLS private key | No | - |
| `TLS_CA_FILE` | Path to CA certificate for client cert verification | No | - |
| `TLS_CLIENT_AUTH` | Client auth mode: `none`, `request`, `require`, `verify-if-given`, `require-verify` | No | `none` |
| `RBAC_POLICY_PATH` | Path to RBAC policy YAML file | No | - |
| `VAULT_ADDR` | Vault server address | No | - |
| `VAULT_TOKEN` | Vault authentication token | No | - |

## Troubleshooting

### "Unauthorized: Invalid credentials"
- Check Authorization header format: `Bearer <token>`
- Verify API key is correct
- For signed keys, ensure `CERBERUS_KEY_<kid>` is set
- Check token expiration with `cerberus.InspectAPIKey()`

### "Forbidden: Insufficient permissions"
- Check user's roles in identity
- Verify RBAC policy allows the action
- Ensure `RBAC_POLICY_PATH` is set correctly

### "no client certificate provided"
- Ensure `TLS_CLIENT_AUTH` is set to `require-verify`
- Verify client certificate is sent in request
- Check certificate is signed by trusted CA

### mTLS not working
- Verify `TLS_CA_FILE` contains the CA that signed client certs
- Check certificate has `clientAuth` extended key usage
- Ensure certificate is not expired
- Test with `openssl s_client -cert ... -key ... -connect ...`

## Examples

See [`tests/integration/auth_test.go`](file:///Users/mohammadrezayousefiha/tartarus/tests/integration/auth_test.go) for comprehensive examples of:
- Simple API key authentication
- Signed API key generation and usage
- mTLS certificate authentication
- RBAC enforcement
- Key rotation procedures
