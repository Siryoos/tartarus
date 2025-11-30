package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// We need a way to simulate a readonly user.
	// Since we can't easily change the server config dynamically to add another key with a different role
	// (unless we restart it with a different config or use OIDC),
	// we will focus on verifying:
	// 1. No Auth -> 401
	// 2. Invalid Auth -> 401
	// 3. Valid Admin Auth -> 200
	//
	// For RBAC, if we could inject a readonly key, we would test it.
	// If the server is started with a multi-key setup (e.g. map), we could test it.
	// But `main.go` currently only takes one `TARTARUS_API_KEY` which gets "admin" role (via NewSimpleAPIKeyAuthenticator).
	// To test RBAC fully, we'd need to modify `main.go` to support loading keys from a file or env var map,
	// or use the OIDC flow which is harder to mock in integration without a provider.
	//
	// However, we CAN test that the admin key HAS access.

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

	t.Run("Valid Admin API Key", func(t *testing.T) {
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

	// To test RBAC properly, we would need a user with restricted permissions.
	// Since we only have one admin key by default, we can't easily test the "deny" path of RBAC
	// unless we change the policy for "admin" role to deny something, which would break other things.
	// Or we could try to access a resource that doesn't exist or is not allowed even for admin?
	// No, admin has allowAll: true.

	// Future: Add support for multiple API keys with different roles in `main.go` or config.
}
