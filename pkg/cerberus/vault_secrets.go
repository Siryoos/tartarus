package cerberus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// VaultConfig holds configuration for Vault client
type VaultConfig struct {
	Address   string        // Vault server address (e.g., https://vault.example.com:8200)
	Token     string        // Vault authentication token
	Namespace string        // Vault namespace (Enterprise feature)
	Timeout   time.Duration // Request timeout
}

// RealVaultSecretProvider resolves secrets from HashiCorp Vault
// Format: vault:path/to/secret:key
// Example: vault:secret/data/myapp:api_key
type RealVaultSecretProvider struct {
	config VaultConfig
	client *http.Client
	cache  map[string]cachedSecret
	mu     sync.RWMutex
	ttl    time.Duration
}

// NewRealVaultSecretProvider creates a new Vault secret provider
func NewRealVaultSecretProvider(config VaultConfig) *RealVaultSecretProvider {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &RealVaultSecretProvider{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		cache: make(map[string]cachedSecret),
		ttl:   15 * time.Minute, // Default TTL for cached secrets
	}
}

// Resolve fetches a secret from Vault
// Format: vault:path/to/secret:key
func (p *RealVaultSecretProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if len(ref) < 7 || ref[:6] != "vault:" {
		return "", fmt.Errorf("unsupported secret reference format: %s", ref)
	}

	// Check cache first
	p.mu.RLock()
	if cached, ok := p.cache[ref]; ok {
		if time.Since(cached.timestamp) < p.ttl {
			p.mu.RUnlock()
			return cached.value, nil
		}
	}
	p.mu.RUnlock()

	// Parse reference: vault:path/to/secret:key
	pathAndKey := ref[6:] // Remove "vault:" prefix
	parts := strings.Split(pathAndKey, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid vault reference format, expected vault:path:key, got: %s", ref)
	}

	secretPath := parts[0]
	secretKey := parts[1]

	// Fetch secret from Vault
	value, err := p.fetchSecret(ctx, secretPath, secretKey)
	if err != nil {
		return "", err
	}

	// Cache the result
	p.mu.Lock()
	p.cache[ref] = cachedSecret{
		value:     value,
		timestamp: time.Now(),
	}
	p.mu.Unlock()

	return value, nil
}

// fetchSecret makes HTTP request to Vault API
func (p *RealVaultSecretProvider) fetchSecret(ctx context.Context, path, key string) (string, error) {
	// Build URL: {address}/v1/{path}
	url := fmt.Sprintf("%s/v1/%s", strings.TrimSuffix(p.config.Address, "/"), path)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create vault request: %w", err)
	}

	// Set authentication token
	req.Header.Set("X-Vault-Token", p.config.Token)

	// Set namespace if configured (Enterprise feature)
	if p.config.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.config.Namespace)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch secret from vault: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("vault returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Data struct {
			Data     map[string]interface{} `json:"data"`
			Metadata map[string]interface{} `json:"metadata"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode vault response: %w", err)
	}

	// Extract the specific key
	value, ok := result.Data.Data[key]
	if !ok {
		return "", fmt.Errorf("key %s not found in vault secret at path %s", key, path)
	}

	// Convert to string
	strValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("secret value for key %s is not a string", key)
	}

	return strValue, nil
}

// ClearCache clears all cached secrets
func (p *RealVaultSecretProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]cachedSecret)
}

// SetTTL sets the cache TTL
func (p *RealVaultSecretProvider) SetTTL(ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ttl = ttl
}
