package cerberus

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RBACPolicyLoader loads RBAC policies from files.
type RBACPolicyLoader struct{}

// NewRBACPolicyLoader creates a new loader.
func NewRBACPolicyLoader() *RBACPolicyLoader {
	return &RBACPolicyLoader{}
}

// LoadPolicies loads policies from a file or directory.
func (l *RBACPolicyLoader) LoadPolicies(path string) (map[string]*RBACPolicy, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	policies := make(map[string]*RBACPolicy)

	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := filepath.Ext(entry.Name())
			if ext != ".yaml" && ext != ".yml" && ext != ".json" {
				continue
			}

			subPolicies, err := l.loadFile(filepath.Join(path, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("failed to load %s: %w", entry.Name(), err)
			}
			for k, v := range subPolicies {
				policies[k] = v
			}
		}
	} else {
		subPolicies, err := l.loadFile(path)
		if err != nil {
			return nil, err
		}
		for k, v := range subPolicies {
			policies[k] = v
		}
	}

	return policies, nil
}

func (l *RBACPolicyLoader) loadFile(path string) (map[string]*RBACPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var policyList []struct {
		Role        string       `yaml:"role" json:"role"`
		Permissions []Permission `yaml:"permissions" json:"permissions"`
	}

	// Try YAML first (superset of JSON)
	if err := yaml.Unmarshal(data, &policyList); err != nil {
		// If it looks like JSON, try JSON explicitly?
		// yaml.Unmarshal usually handles JSON too.
		// Let's try direct map if list fails?
		// Actually, let's support a map format too: role -> permissions
		var policyMap map[string][]Permission
		if err2 := yaml.Unmarshal(data, &policyMap); err2 == nil {
			result := make(map[string]*RBACPolicy)
			for role, perms := range policyMap {
				result[role] = &RBACPolicy{
					Role:        role,
					Permissions: perms,
				}
			}
			return result, nil
		}
		return nil, fmt.Errorf("failed to parse policy file: %w", err)
	}

	result := make(map[string]*RBACPolicy)
	for _, p := range policyList {
		result[p.Role] = &RBACPolicy{
			Role:        p.Role,
			Permissions: p.Permissions,
		}
	}

	return result, nil
}
