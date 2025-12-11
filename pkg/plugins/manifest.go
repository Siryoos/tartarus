package plugins

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest describes plugin metadata loaded from manifest.yaml.
type Manifest struct {
	// APIVersion for future compatibility.
	APIVersion string `yaml:"apiVersion"`

	// Kind should be "TartarusPlugin".
	Kind string `yaml:"kind"`

	// Metadata contains plugin identity.
	Metadata ManifestMetadata `yaml:"metadata"`

	// Spec contains plugin configuration.
	Spec ManifestSpec `yaml:"spec"`
}

// ManifestMetadata contains plugin identity fields.
type ManifestMetadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Author      string `yaml:"author"`
	Description string `yaml:"description"`
}

// ManifestSpec contains plugin runtime configuration.
type ManifestSpec struct {
	// Type is "judge" or "fury".
	Type PluginType `yaml:"type"`

	// EntryPoint is the .so file name (Linux) or path.
	EntryPoint string `yaml:"entryPoint"`

	// Config is plugin-specific configuration passed to Init().
	Config map[string]any `yaml:"config"`

	// Dependencies lists required Tartarus versions.
	Dependencies []string `yaml:"dependencies"`
}

// LoadManifest reads a manifest.yaml file and returns the parsed Manifest.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

// Validate checks that required fields are present.
func (m *Manifest) Validate() error {
	if m.APIVersion == "" {
		return fmt.Errorf("manifest missing apiVersion")
	}
	if m.Kind != "TartarusPlugin" {
		return fmt.Errorf("manifest kind must be 'TartarusPlugin', got '%s'", m.Kind)
	}
	if m.Metadata.Name == "" {
		return fmt.Errorf("manifest missing metadata.name")
	}
	if m.Metadata.Version == "" {
		return fmt.Errorf("manifest missing metadata.version")
	}
	if m.Spec.Type != PluginTypeJudge && m.Spec.Type != PluginTypeFury {
		return fmt.Errorf("manifest spec.type must be 'judge' or 'fury', got '%s'", m.Spec.Type)
	}
	if m.Spec.EntryPoint == "" {
		return fmt.Errorf("manifest missing spec.entryPoint")
	}
	return nil
}
