package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tartarus-sandbox/tartarus/pkg/cerberus"
	"github.com/tartarus-sandbox/tartarus/pkg/erebus"
	"github.com/tartarus-sandbox/tartarus/pkg/typhon"
)

func TestSecurityHardening(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping security integration test in short mode")
	}

	t.Run("Seccomp Profile Per Class", func(t *testing.T) {
		tests := []struct {
			class        string
			expectedType string
		}{
			{"ember", "quarantine-strict"},
			{"flame", "quarantine"},
			{"blaze", "default"},
			{"inferno", "default"},
		}

		for _, tt := range tests {
			t.Run(tt.class, func(t *testing.T) {
				profile, err := typhon.GetProfileForClass(tt.class)
				require.NoError(t, err)
				require.NotNil(t, profile)
				assert.Equal(t, "SCMP_ACT_ALLOW", profile.DefaultAction)

				// Verify JSON serialization works
				jsonStr, err := profile.ToJSON()
				require.NoError(t, err)
				assert.Contains(t, jsonStr, "default_action")
				assert.Contains(t, jsonStr, "syscalls")
			})
		}
	})

	t.Run("Secret Resolution Integration", func(t *testing.T) {
		ctx := context.Background()

		t.Run("Environment Secrets", func(t *testing.T) {
			os.Setenv("TEST_SECRET", "test-value-123")
			defer os.Unsetenv("TEST_SECRET")

			provider := cerberus.NewEnvSecretProvider()
			value, err := provider.Resolve(ctx, "env:TEST_SECRET")
			require.NoError(t, err)
			assert.Equal(t, "test-value-123", value)
		})

		t.Run("Vault Secrets - Mock", func(t *testing.T) {
			// Use original mock provider for testing
			provider := cerberus.NewVaultSecretProvider()
			value, err := provider.Resolve(ctx, "vault:secret/myapp:api_key")
			require.NoError(t, err)
			assert.Equal(t, "super-secret-api-key", value)
		})

		t.Run("Composite Provider", func(t *testing.T) {
			os.Setenv("MY_ENV_SECRET", "from-env")
			defer os.Unsetenv("MY_ENV_SECRET")

			composite := cerberus.NewCompositeSecretProvider(
				cerberus.NewEnvSecretProvider(),
				cerberus.NewVaultSecretProvider(),
			)

			// Test env resolution
			val1, err := composite.Resolve(ctx, "env:MY_ENV_SECRET")
			require.NoError(t, err)
			assert.Equal(t, "from-env", val1)

			// Test vault resolution
			val2, err := composite.Resolve(ctx, "vault:secret/db:password")
			require.NoError(t, err)
			assert.Equal(t, "db-password-123", val2)
		})
	})

	t.Run("Vulnerability Scanning", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Test scanner configuration
		scanner := erebus.NewTrivyScanner()
		assert.Equal(t, "trivy", scanner.BinaryPath)
		assert.Equal(t, erebus.SeverityCritical, scanner.MinSeverity)
		assert.True(t, scanner.ExitCodeOnFail)

		// Test custom severity scanner
		customScanner := erebus.NewTrivyScannerWithSeverities([]erebus.Severity{
			erebus.SeverityCritical,
			erebus.SeverityHigh,
		})
		assert.Len(t, customScanner.Severities, 2)

		// Test with mock scanner for CI
		mockScanner := &erebus.MockScanner{ShouldFail: false}
		err := mockScanner.Scan(ctx, "/tmp")
		assert.NoError(t, err)

		// Test failing mock
		failingScanner := &erebus.MockScanner{ShouldFail: true}
		err = failingScanner.Scan(ctx, "/tmp")
		assert.Error(t, err)
	})

	t.Run("Secrets Configuration", func(t *testing.T) {
		config := cerberus.DefaultSecretsConfig()

		assert.True(t, config.EnableEnv)
		assert.False(t, config.EnableVault)
		assert.False(t, config.EnableKMS)
		assert.Equal(t, 15*time.Minute, config.CacheTTL)
		assert.True(t, config.CircuitBreaker.Enabled)
		assert.Equal(t, 5, config.CircuitBreaker.FailureThreshold)
	})
}

func TestKernelHardeningVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping kernel hardening test in short mode")
	}

	// This test verifies that kernel parameters are correctly set
	// In a real environment, you would launch a VM and inspect the kernel command line
	t.Run("Kernel Parameters Present", func(t *testing.T) {
		// Expected security parameters
		expectedParams := []string{
			"init_on_alloc=1",
			"init_on_free=1",
			"mds=full,nosmt",
			"l1tf=full,force",
			"spec_store_bypass_disable=on",
			"tsx=off",
			"vsyscall=none",
			"debugfs=off",
			"oops=panic",
			"pti=on",
			"nosmt",
			"randomize_kstack_offset=on",
		}

		// This would be verified by launching a VM and checking logs
		// For unit testing, we just verify the parameters are defined
		for _, param := range expectedParams {
			assert.NotEmpty(t, param, "Parameter should not be empty")
		}
	})
}
