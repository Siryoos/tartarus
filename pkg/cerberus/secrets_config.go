package cerberus

import (
	"time"
)

// SecretsConfig holds configuration for secrets management
type SecretsConfig struct {
	// Provider selection
	EnableVault bool
	EnableKMS   bool
	EnableEnv   bool // Always available as fallback

	// Vault configuration
	Vault VaultConfig

	// KMS configuration (region determined from AWS config)
	KMSCacheTTL time.Duration

	// Global settings
	CacheTTL       time.Duration // Default TTL for cached secrets
	CircuitBreaker CircuitBreakerConfig
	MaxRetries     int
	RetryBackoff   time.Duration
}

// CircuitBreakerConfig defines circuit breaker settings for external secret providers
type CircuitBreakerConfig struct {
	Enabled          bool
	FailureThreshold int           // Number of failures before opening circuit
	SuccessThreshold int           // Number of successes to close circuit
	Timeout          time.Duration // Time to wait before attempting to close circuit
}

// DefaultSecretsConfig returns default configuration
func DefaultSecretsConfig() SecretsConfig {
	return SecretsConfig{
		EnableEnv:   true,
		EnableVault: false,
		EnableKMS:   false,
		Vault: VaultConfig{
			Address: "http://localhost:8200",
			Timeout: 10 * time.Second,
		},
		KMSCacheTTL: 15 * time.Minute,
		CacheTTL:    15 * time.Minute,
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          30 * time.Second,
		},
		MaxRetries:   3,
		RetryBackoff: 1 * time.Second,
	}
}
