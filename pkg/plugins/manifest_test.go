package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifest_LoadAndValidate(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "ValidJudgeManifest",
			content: `apiVersion: v1
kind: TartarusPlugin
metadata:
  name: rate-limit-judge
  version: 1.0.0
  author: Tartarus Team
  description: Rate limiting judge
spec:
  type: judge
  entryPoint: rate-limit.so
  config:
    maxRPS: 100
`,
			wantErr: false,
		},
		{
			name: "ValidFuryManifest",
			content: `apiVersion: v1
kind: TartarusPlugin
metadata:
  name: cost-aware-fury
  version: 2.0.0
  author: Community
  description: Cost monitoring fury
spec:
  type: fury
  entryPoint: cost-aware.so
`,
			wantErr: false,
		},
		{
			name: "MissingAPIVersion",
			content: `kind: TartarusPlugin
metadata:
  name: test
  version: 1.0.0
spec:
  type: judge
  entryPoint: test.so
`,
			wantErr:   true,
			errSubstr: "apiVersion",
		},
		{
			name: "WrongKind",
			content: `apiVersion: v1
kind: WrongKind
metadata:
  name: test
  version: 1.0.0
spec:
  type: judge
  entryPoint: test.so
`,
			wantErr:   true,
			errSubstr: "TartarusPlugin",
		},
		{
			name: "MissingName",
			content: `apiVersion: v1
kind: TartarusPlugin
metadata:
  version: 1.0.0
spec:
  type: judge
  entryPoint: test.so
`,
			wantErr:   true,
			errSubstr: "metadata.name",
		},
		{
			name: "InvalidType",
			content: `apiVersion: v1
kind: TartarusPlugin
metadata:
  name: test
  version: 1.0.0
spec:
  type: invalid
  entryPoint: test.so
`,
			wantErr:   true,
			errSubstr: "spec.type",
		},
		{
			name: "MissingEntryPoint",
			content: `apiVersion: v1
kind: TartarusPlugin
metadata:
  name: test
  version: 1.0.0
spec:
  type: judge
`,
			wantErr:   true,
			errSubstr: "spec.entryPoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write temp manifest
			tmpDir := t.TempDir()
			manifestPath := filepath.Join(tmpDir, "manifest.yaml")
			if err := os.WriteFile(manifestPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test manifest: %v", err)
			}

			manifest, err := LoadManifest(manifestPath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tt.errSubstr)
				} else if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if manifest == nil {
					t.Error("expected manifest to be non-nil")
				}
			}
		})
	}
}

func TestManifest_ParseConfig(t *testing.T) {
	content := `apiVersion: v1
kind: TartarusPlugin
metadata:
  name: test-config
  version: 1.0.0
spec:
  type: judge
  entryPoint: test.so
  config:
    maxRPS: 100
    enabled: true
    tags:
      - production
      - security
`
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test manifest: %v", err)
	}

	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	// Check config values
	if maxRPS, ok := manifest.Spec.Config["maxRPS"].(int); !ok || maxRPS != 100 {
		t.Errorf("expected maxRPS=100, got %v", manifest.Spec.Config["maxRPS"])
	}
	if enabled, ok := manifest.Spec.Config["enabled"].(bool); !ok || !enabled {
		t.Errorf("expected enabled=true, got %v", manifest.Spec.Config["enabled"])
	}
	if tags, ok := manifest.Spec.Config["tags"].([]any); !ok || len(tags) != 2 {
		t.Errorf("expected tags with 2 items, got %v", manifest.Spec.Config["tags"])
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
