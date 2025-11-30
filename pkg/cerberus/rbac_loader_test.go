package cerberus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRBACPolicies(t *testing.T) {
	// Create a temporary directory for policies
	tmpDir, err := os.MkdirTemp("", "rbac-policies")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a valid policy file
	policyContent := `
- role: admin
  permissions:
    - allowAll: true
- role: viewer
  permissions:
    - actions: ["read"]
      resources: ["sandbox"]
`
	err = os.WriteFile(filepath.Join(tmpDir, "policy.yaml"), []byte(policyContent), 0644)
	require.NoError(t, err)

	// Create another policy file (JSON)
	jsonContent := `
[
  {
    "role": "editor",
    "permissions": [
      {
        "actions": ["read", "update"],
        "resources": ["sandbox"]
      }
    ]
  }
]
`
	err = os.WriteFile(filepath.Join(tmpDir, "policy.json"), []byte(jsonContent), 0644)
	require.NoError(t, err)

	// Load policies
	loader := NewRBACPolicyLoader()
	policies, err := loader.LoadPolicies(tmpDir)
	require.NoError(t, err)

	// Verify loaded policies
	assert.Len(t, policies, 3)

	adminPolicy, ok := policies["admin"]
	require.True(t, ok)
	assert.True(t, adminPolicy.Permissions[0].AllowAll)

	viewerPolicy, ok := policies["viewer"]
	require.True(t, ok)
	assert.Equal(t, Action("read"), viewerPolicy.Permissions[0].Actions[0])

	editorPolicy, ok := policies["editor"]
	require.True(t, ok)
	assert.Equal(t, Action("read"), editorPolicy.Permissions[0].Actions[0])
	assert.Equal(t, Action("update"), editorPolicy.Permissions[0].Actions[1])
}

func TestLoadRBACPolicies_InvalidFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rbac-invalid")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte("invalid yaml content: :"), 0644)
	require.NoError(t, err)

	loader := NewRBACPolicyLoader()
	_, err = loader.LoadPolicies(tmpDir)
	assert.Error(t, err)
}
