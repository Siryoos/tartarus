package cerberus

import (
	"context"
	"fmt"
	"os"
)

// SecretProvider resolves secret references to values
type SecretProvider interface {
	// Resolve returns the secret value for a given reference
	Resolve(ctx context.Context, ref string) (string, error)
}

// EnvSecretProvider resolves secrets from environment variables
// Format: env:VAR_NAME
type EnvSecretProvider struct{}

func NewEnvSecretProvider() *EnvSecretProvider {
	return &EnvSecretProvider{}
}

func (p *EnvSecretProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if len(ref) > 4 && ref[:4] == "env:" {
		key := ref[4:]
		val := os.Getenv(key)
		if val == "" {
			return "", fmt.Errorf("secret environment variable not found: %s", key)
		}
		return val, nil
	}

	return "", fmt.Errorf("unsupported secret reference format: %s", ref)
}

// VaultSecretProvider resolves secrets from a (mocked) Vault
// Format: vault:path/to/secret:key
type VaultSecretProvider struct {
	// In a real implementation, this would hold the Vault client
	data map[string]string
}

func NewVaultSecretProvider() *VaultSecretProvider {
	return &VaultSecretProvider{
		data: map[string]string{
			"secret/myapp:api_key": "super-secret-api-key",
			"secret/db:password":   "db-password-123",
		},
	}
}

func (p *VaultSecretProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if len(ref) > 6 && ref[:6] == "vault:" {
		key := ref[6:]
		val, ok := p.data[key]
		if !ok {
			return "", fmt.Errorf("secret not found in vault: %s", key)
		}
		return val, nil
	}
	return "", fmt.Errorf("unsupported secret reference format: %s", ref)
}

// CompositeSecretProvider chains multiple providers
type CompositeSecretProvider struct {
	providers []SecretProvider
}

func NewCompositeSecretProvider(providers ...SecretProvider) *CompositeSecretProvider {
	return &CompositeSecretProvider{providers: providers}
}

func (p *CompositeSecretProvider) Resolve(ctx context.Context, ref string) (string, error) {
	for _, provider := range p.providers {
		val, err := provider.Resolve(ctx, ref)
		if err == nil {
			return val, nil
		}
		// If error is "unsupported format", continue.
		// But our providers return error if format matches but key not found?
		// Or if format doesn't match.
		// Let's assume providers check prefix.
	}
	return "", fmt.Errorf("failed to resolve secret %s", ref)
}
