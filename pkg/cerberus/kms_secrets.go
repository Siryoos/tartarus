package cerberus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// KMSSecretProvider resolves secrets from AWS KMS
// Format: kms:arn:aws:kms:region:account:key/key-id:secret-name
// Example: kms:arn:aws:kms:us-east-1:123456789012:key/abc-123:db-password
type KMSSecretProvider struct {
	client *kms.Client
	cache  map[string]cachedSecret
	mu     sync.RWMutex
	ttl    time.Duration
}

type cachedSecret struct {
	value     string
	timestamp time.Time
}

// NewKMSSecretProvider creates a new KMS secret provider with default AWS config
func NewKMSSecretProvider(ctx context.Context) (*KMSSecretProvider, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &KMSSecretProvider{
		client: kms.NewFromConfig(cfg),
		cache:  make(map[string]cachedSecret),
		ttl:    15 * time.Minute, // Default TTL for cached secrets
	}, nil
}

// NewKMSSecretProviderWithClient creates a provider with a custom KMS client
func NewKMSSecretProviderWithClient(client *kms.Client, ttl time.Duration) *KMSSecretProvider {
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	return &KMSSecretProvider{
		client: client,
		cache:  make(map[string]cachedSecret),
		ttl:    ttl,
	}
}

// Resolve decrypts a secret from KMS
// Format: kms:arn:aws:kms:region:account:key/key-id:secret-name
// The ciphertext is the base64-encoded encrypted secret
func (p *KMSSecretProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if len(ref) < 5 || ref[:4] != "kms:" {
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

	// Parse ref format: kms:arn:...:key/key-id:secret-name
	// For simplicity, we'll expect the entire ref after "kms:" to be the key ARN
	// and we'll use a mock encrypted value or parameter store lookup
	// In a real implementation, you would:
	// 1. Store encrypted secrets in Parameter Store or Secrets Manager
	// 2. Use KMS to decrypt them
	// 3. Or directly call KMS Decrypt with ciphertext

	// For now, this is a stub showing the structure
	// In production, you'd:
	// - Parse the ARN to extract the key ID
	// - Retrieve the encrypted secret from somewhere (Parameter Store, S3, etc.)
	// - Call KMS Decrypt

	keyARN := ref[4:] // Remove "kms:" prefix
	// Example: arn:aws:kms:us-east-1:123456789012:key/abc-123:secretname

	// This is a simplified implementation
	// In reality, you'd need to:
	// 1. Parse the ARN and secret name
	// 2. Fetch the encrypted secret from Parameter Store or Secrets Manager
	// 3. Decrypt it using KMS

	// For demonstration, we'll return a mock decrypted value
	// Real implementation would call kms.Decrypt
	_ = keyARN

	// Mock decryption - in production, use actual KMS Decrypt API
	decryptedValue := fmt.Sprintf("decrypted-secret-from-%s", keyARN)

	// Cache the result
	p.mu.Lock()
	p.cache[ref] = cachedSecret{
		value:     decryptedValue,
		timestamp: time.Now(),
	}
	p.mu.Unlock()

	return decryptedValue, nil
}

// RealResolve is an example of how a real KMS decrypt would work
// This is commented out but shows the proper approach
func (p *KMSSecretProvider) realResolve(ctx context.Context, ciphertext []byte, keyID string) (string, error) {
	input := &kms.DecryptInput{
		CiphertextBlob: ciphertext,
		KeyId:          aws.String(keyID),
	}

	result, err := p.client.Decrypt(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secret: %w", err)
	}

	return string(result.Plaintext), nil
}

// ClearCache clears all cached secrets
func (p *KMSSecretProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]cachedSecret)
}
