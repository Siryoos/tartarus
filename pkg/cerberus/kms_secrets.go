package cerberus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// KMSSecretProvider resolves secrets from AWS SSM Parameter Store (which uses KMS)
// Format: kms:parameter-name
// Example: kms:/tartarus/prod/db-password
type KMSSecretProvider struct {
	client *ssm.Client
	cache  map[string]cachedSecret
	mu     sync.RWMutex
	ttl    time.Duration
}

type cachedSecret struct {
	value     string
	timestamp time.Time
}

// NewKMSSecretProvider creates a new KMS secret provider with default AWS config
func NewKMSSecretProvider(ctx context.Context, region string) (*KMSSecretProvider, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &KMSSecretProvider{
		client: ssm.NewFromConfig(cfg),
		cache:  make(map[string]cachedSecret),
		ttl:    15 * time.Minute, // Default TTL for cached secrets
	}, nil
}

// NewKMSSecretProviderWithClient creates a provider with a custom SSM client
func NewKMSSecretProviderWithClient(client *ssm.Client, ttl time.Duration) *KMSSecretProvider {
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	return &KMSSecretProvider{
		client: client,
		cache:  make(map[string]cachedSecret),
		ttl:    ttl,
	}
}

// Resolve fetches a secret from AWS SSM Parameter Store
// Format: kms:parameter-name
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

	// Parse ref format: kms:parameter-name
	paramName := ref[4:] // Remove "kms:" prefix

	// Fetch from SSM
	input := &ssm.GetParameterInput{
		Name:           aws.String(paramName),
		WithDecryption: aws.Bool(true),
	}

	result, err := p.client.GetParameter(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to fetch parameter %s: %w", paramName, err)
	}

	if result.Parameter == nil || result.Parameter.Value == nil {
		return "", fmt.Errorf("parameter %s not found or empty", paramName)
	}

	value := *result.Parameter.Value

	// Cache the result
	p.mu.Lock()
	p.cache[ref] = cachedSecret{
		value:     value,
		timestamp: time.Now(),
	}
	p.mu.Unlock()

	return value, nil
}

// ClearCache clears all cached secrets
func (p *KMSSecretProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]cachedSecret)
}
